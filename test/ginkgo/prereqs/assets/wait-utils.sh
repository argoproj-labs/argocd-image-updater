#!/bin/bash
# Assisted-by: Claude AI model
# Utility functions for waiting on cluster and registry readiness

# wait_for_cluster_ready waits for the Kubernetes cluster to be ready
# Usage: wait_for_cluster_ready [timeout_seconds] [kubectl_command]
wait_for_cluster_ready() {
	local timeout=${1:-120}
	local kubectl=${2:-kubectl}
	local elapsed=0

	echo "--- Waiting for k3d cluster to be ready ---"
	while [ $elapsed -lt $timeout ]; do
		if $kubectl get nodes >/dev/null 2>&1 && $kubectl get namespaces >/dev/null 2>&1; then
			echo "Cluster is ready"
			return 0
		fi
		printf "."
		sleep 2
		elapsed=$((elapsed + 2))
	done
	echo ""
	echo "Error: Cluster did not become ready within $timeout seconds" >&2
	return 1
}

# wait_for_registry_ready waits for the registry service to be accessible
# Usage: wait_for_registry_ready [timeout_seconds] [registry_url]
wait_for_registry_ready() {
	local timeout=${1:-60}
	local registry_url=${2:-https://127.0.0.1:30000/v2/}
	local elapsed=0

	echo "Waiting for registry service to be accessible at 127.0.0.1:30000..."
	while [ $elapsed -lt $timeout ]; do
		if curl -k -s -o /dev/null -w "%{http_code}" "$registry_url" 2>/dev/null | grep -q "200\|401\|404"; then
			echo "Registry is accessible"
			return 0
		fi
		printf "."
		sleep 1
		elapsed=$((elapsed + 1))
	done
	echo ""
	echo "Warning: Registry may not be fully ready, but attempting push anyway..."
	return 1
}

# If script is called directly (not sourced), execute the function
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
	"$@"
fi

