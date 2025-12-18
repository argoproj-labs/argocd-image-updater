#!/bin/bash
# Assisted-by: Claude AI model

set -e

REGISTRY="127.0.0.1:30000"
DAEMON_JSON="/etc/docker/daemon.json"
SUDO_CMD=""

# Check if we're in CI (GitHub Actions) - always configure in CI
IS_CI="${CI:-false}"
if [ "$GITHUB_ACTIONS" = "true" ] || [ "$IS_CI" = "true" ]; then
	IS_CI="true"
fi

# For local testing: check if registry is already configured and skip if so
if [ "$IS_CI" != "true" ]; then
	# Try to find daemon.json in common locations
	CHECK_DAEMON_JSON=""
	if [ -f "/etc/docker/daemon.json" ]; then
		CHECK_DAEMON_JSON="/etc/docker/daemon.json"
		CHECK_SUDO="sudo"
	elif [ -f "$HOME/.docker/daemon.json" ]; then
		CHECK_DAEMON_JSON="$HOME/.docker/daemon.json"
		CHECK_SUDO=""
	fi
	
	# If daemon.json exists, check if registry is already configured
	if [ -n "$CHECK_DAEMON_JSON" ] && [ -f "$CHECK_DAEMON_JSON" ]; then
		if command -v jq >/dev/null 2>&1; then
			EXISTING_REGISTRIES=$(${CHECK_SUDO} cat "$CHECK_DAEMON_JSON" 2>/dev/null | jq -r '.["insecure-registries"] // [] | .[]' 2>/dev/null || echo "")
			if echo "$EXISTING_REGISTRIES" | grep -q "^${REGISTRY}$"; then
				echo "Registry ${REGISTRY} is already configured in ${CHECK_DAEMON_JSON}"
				echo "Skipping Docker daemon configuration (local testing only)"
				exit 0
			fi
		fi
	fi
fi

# Detect OS and set daemon.json path
if [ "$(uname)" = "Darwin" ]; then
	echo "Detected macOS with Docker Desktop"
	echo "Note: Docker Desktop on macOS uses its own settings UI."
	echo "Please manually configure insecure registry in Docker Desktop:"
	echo "  1. Open Docker Desktop"
	echo "  2. Go to Settings > Docker Engine"
	echo "  3. Add \"insecure-registries\": [\"127.0.0.1:30000\"] to the JSON config"
	echo "  4. Click Apply & Restart"
	echo ""
	echo "Alternatively, if you have Docker running via Colima or similar, the script will attempt to configure it."
	# Try common macOS Docker daemon.json locations
	if [ -f "/etc/docker/daemon.json" ]; then
		DAEMON_JSON="/etc/docker/daemon.json"
		SUDO_CMD="sudo"
	elif [ -f "$HOME/.docker/daemon.json" ]; then
		DAEMON_JSON="$HOME/.docker/daemon.json"
	fi
else
	echo "Detected Linux, using system daemon.json: $DAEMON_JSON"
	# Always use sudo for /etc/docker on Linux (system directory)
	# Also check if file exists and is not writable, or directory is not writable
	if [ "$(dirname "$DAEMON_JSON")" = "/etc/docker" ] || \
	   ([ -f "$DAEMON_JSON" ] && [ ! -w "$DAEMON_JSON" ]) || \
	   [ ! -w "$(dirname "$DAEMON_JSON")" ] 2>/dev/null; then
		SUDO_CMD="sudo"
	fi
	if [ ! -d "$(dirname "$DAEMON_JSON")" ]; then
		${SUDO_CMD} mkdir -p "$(dirname "$DAEMON_JSON")"
	fi
fi

# Check if we can proceed with file-based configuration
if [ "$(uname)" = "Darwin" ] && [ ! -f "$DAEMON_JSON" ] && [ ! -w "/etc/docker" ]; then
	echo "Skipping file-based configuration on macOS (Docker Desktop requires manual configuration)"
	exit 0
fi

# Create backup if file exists
if [ -f "$DAEMON_JSON" ]; then
	echo "Backing up existing daemon.json to $DAEMON_JSON.backup"
	${SUDO_CMD} cp "$DAEMON_JSON" "$DAEMON_JSON.backup" || true
fi

# Read existing config or create empty object
if [ -f "$DAEMON_JSON" ]; then
	EXISTING_CONFIG=$(${SUDO_CMD} cat "$DAEMON_JSON" 2>/dev/null || echo "{}")
else
	EXISTING_CONFIG="{}"
fi

# Use jq to merge insecure-registries (jq is lightweight and commonly available)
if command -v jq >/dev/null 2>&1; then
	NEW_CONFIG=$(echo "$EXISTING_CONFIG" | jq --arg reg "$REGISTRY" 'if .["insecure-registries"] == null then .["insecure-registries"] = [] else . end | if (.["insecure-registries"] | index($reg)) == null then .["insecure-registries"] += [$reg] else . end')
else
	echo "Error: jq is required for JSON manipulation but was not found."
	exit 1
fi

# Write new config
echo "$NEW_CONFIG" | ${SUDO_CMD} tee "$DAEMON_JSON" > /dev/null
echo "Successfully updated $DAEMON_JSON"

# Restart Docker if on Linux and we have systemd
if [ "$(uname)" != "Darwin" ] && command -v systemctl >/dev/null 2>&1; then
	echo "Restarting Docker daemon..."
	${SUDO_CMD} systemctl restart docker || echo "Warning: Could not restart Docker. Please restart manually."
	sleep 2
	docker info > /dev/null 2>&1 || echo "Warning: Docker may need a moment to restart"
elif [ "$(uname)" = "Darwin" ] && [ -f "$DAEMON_JSON" ]; then
	echo "On macOS, please restart Docker Desktop manually for the changes to take effect:"
	echo "  1. Click the Docker icon in the menu bar"
	echo "  2. Select 'Restart' from the dropdown menu"
	echo "  Or restart Docker Desktop from the Applications folder"
fi

