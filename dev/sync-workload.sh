#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=dev/common.sh
source "${ROOT_DIR}/dev/common.sh"

NAMESPACE="${NAMESPACE:-argocd}"
APP_NAME="${APP_NAME:-demo-app}"
DEPLOYMENT="${DEPLOYMENT:-demo-app}"
DEPLOY_NS="${DEPLOY_NS:-default}"

IMAGE="$(kubectl -n "$NAMESPACE" get application "$APP_NAME" \
  -o jsonpath='{.spec.source.kustomize.images[0]}' 2>/dev/null || true)"

if [[ -z "$IMAGE" ]]; then
  echo "No kustomize image override found on Application/$APP_NAME" >&2
  exit 1
fi

# kustomize.images entries look like "demo-app=registry/repo:tag" or "repo:tag@digest"
if [[ "$IMAGE" == *"="* ]]; then
  IMAGE="${IMAGE#*=}"
fi

# kind loads by repo:tag; strip @digest for the container image reference.
PULL_IMAGE="${IMAGE%%@*}"

echo "Syncing deployment to fake ECR image: ${PULL_IMAGE}"
kubectl -n "$DEPLOY_NS" set image "deployment/${DEPLOYMENT}" "demo=${PULL_IMAGE}"
kubectl -n "$DEPLOY_NS" patch deployment "${DEPLOYMENT}" --type=strategic \
  -p '{"spec":{"template":{"spec":{"containers":[{"name":"demo","imagePullPolicy":"IfNotPresent"}]}}}}'
kubectl -n "$DEPLOY_NS" rollout status "deployment/${DEPLOYMENT}" --timeout=60s
kubectl -n "$DEPLOY_NS" get pods -l app=demo-app -o wide
