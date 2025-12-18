#!/bin/bash

# Initializes Nginx and the git cgi scripts
# for git http-backend through fcgiwrap.
#
# Usage:
#   entrypoint <commands>

set -o errexit
set -o pipefail

readonly GIT_PROJECT_ROOT="/var/lib/git"
readonly GIT_INITIAL_ROOT="/var/lib/initial"
export GIT_HTTP_EXPORT_ALL="true"
readonly GIT_USER="git"
readonly GIT_GROUP="git"

readonly FCGIPROGRAM="/usr/bin/fcgiwrap"
readonly USERID="nginx"
readonly SOCKUSERID="$USERID"
readonly FCGISOCKET="/var/run/fcgiwrap.socket"
readonly SSL_CERT_PATH="/etc/nginx/localhost.crt"
readonly SSL_KEY_PATH="/etc/nginx/localhost.key"
readonly HTPASSWD_PATH="/etc/nginx/htpasswd"
readonly GIT_USERNAME="git"
readonly GIT_PASSWORD="git"

main() {
  mkdir -p "$GIT_PROJECT_ROOT"

  # Generate self-signed certificate if it doesn't exist
  generate_ssl_certificate

  # Generate htpasswd file if it doesn't exist
  generate_htpasswd

  # Checks if $GIT_INITIAL_ROOT has files
  if [[ $(ls -A "${GIT_INITIAL_ROOT}") ]]; then
    initialize_initial_repositories
  fi
  initialize_services
}

generate_ssl_certificate() {
  # Generate self-signed certificate if it doesn't exist
  if [[ ! -f "$SSL_CERT_PATH" ]] || [[ ! -f "$SSL_KEY_PATH" ]]; then
    echo "Generating self-signed SSL certificate..."
    openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
      -keyout "$SSL_KEY_PATH" \
      -out "$SSL_CERT_PATH" \
      -subj "/C=US/ST=State/L=City/O=Organization/CN=localhost" \
      2>/dev/null
    chmod 600 "$SSL_KEY_PATH"
    chmod 644 "$SSL_CERT_PATH"
    echo "Self-signed SSL certificate generated at $SSL_CERT_PATH"
  fi
}

generate_htpasswd() {
  # Generate htpasswd file if it doesn't exist
  if [[ ! -f "$HTPASSWD_PATH" ]]; then
    echo "Generating htpasswd file..."
    htpasswd -b -c "$HTPASSWD_PATH" "$GIT_USERNAME" "$GIT_PASSWORD" 2>/dev/null
    chmod 644 "$HTPASSWD_PATH"
    echo "htpasswd file generated at $HTPASSWD_PATH for user $GIT_USERNAME"
  fi
}

initialize_services() {
  # Check permissions on $GIT_PROJECT_ROOT
  chown -R nginx:git "$GIT_PROJECT_ROOT"
  chmod -R 775 "$GIT_PROJECT_ROOT"

  /usr/bin/spawn-fcgi \
    -s "$FCGISOCKET" \
    -F 4 \
    -u "$USERID" \
    -g "$USERID" \
    -U "$USERID" \
    -G "$GIT_GROUP" -- \
    "$FCGIPROGRAM"
  exec nginx
}

initialize_initial_repositories() {
  cd "$GIT_INITIAL_ROOT"
  find . -type d -maxdepth 1 -mindepth 1 -print0 | while IFS= read -r -d '' dir; do
    echo "Initializing repository $dir"
    init_and_commit "$dir"
  done
}

init_and_commit() {
  local dir="$1"
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' RETURN
  cp -r "$dir"/. "$tmp_dir"
  pushd . >/dev/null
  cd "$tmp_dir"
  if [[ -d "./.git" ]]; then
    rm -rf ./.git
  fi

  git init &>/dev/null
  git add --all . &>/dev/null
  git commit -m "first commit" &>/dev/null
  local repo_name="${dir#./}"
  git clone --bare "$tmp_dir" "$GIT_PROJECT_ROOT/${repo_name}.git" &>/dev/null

  popd >/dev/null
}

main "$@"
