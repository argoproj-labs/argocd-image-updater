#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=dev/common.sh
source "${ROOT_DIR}/dev/common.sh"

APP_NAMESPACE="${APP_NAMESPACE:-argocd}"
APP_NAME="${APP_NAME:-demo-app}"
CONTROLLER_NAMESPACE="${CONTROLLER_NAMESPACE:-argocd}"
CONTROLLER_DEPLOY="${CONTROLLER_DEPLOY:-argocd-image-updater-controller}"

require_cluster() {
  if ! kubectl cluster-info >/dev/null 2>&1; then
    echo "kubectl is not connected to a cluster. Run dev/setup.sh first." >&2
    exit 1
  fi
  if ! kubectl -n "$CONTROLLER_NAMESPACE" get deploy "$CONTROLLER_DEPLOY" >/dev/null 2>&1; then
    echo "Controller deployment not found. Run dev/setup.sh first." >&2
    exit 1
  fi
}

get_app_kustomize_image() {
  kubectl -n "$APP_NAMESPACE" get application "$APP_NAME" \
    -o jsonpath='{.spec.source.kustomize.images[0]}' 2>/dev/null || true
}

wait_for_app_image_match() {
  local pattern="$1"
  local timeout="${2:-60}"
  local current=""
  local i

  for ((i = 1; i <= timeout; i++)); do
    current="$(get_app_kustomize_image)"
    if [[ "$current" == *"$pattern"* ]]; then
      echo "$current"
      return 0
    fi
    sleep 1
  done

  echo "Timed out waiting for Application image to match ${pattern}. Last value: ${current:-<empty>}" >&2
  return 1
}

wait_for_stable_app_image() {
  local expected="$1"
  local timeout="${2:-20}"
  local current=""
  local i

  for ((i = 1; i <= timeout; i++)); do
    current="$(get_app_kustomize_image)"
    if [[ "$current" == "$expected" ]]; then
      return 0
    fi
    sleep 1
  done

  echo "Application image changed unexpectedly." >&2
  echo "  expected: ${expected}" >&2
  echo "  current : ${current:-<empty>}" >&2
  return 1
}

send_ecr_event() {
  local tag="$1"
  local digest="$2"
  local event_time="${3:-}"
  local repository="${4:-$ECR_REPO}"

  local -a env_args=(IMAGE_TAG="$tag" IMAGE_DIGEST="$digest" REPOSITORY="$repository")
  if [[ -n "$event_time" ]]; then
    env_args+=(EVENT_TIME="$event_time")
  fi

  env "${env_args[@]}" "${ROOT_DIR}/dev/send-test-event.sh"
}

pass() {
  echo "PASS: $*"
}

fail() {
  echo "FAIL: $*" >&2
  exit 1
}
