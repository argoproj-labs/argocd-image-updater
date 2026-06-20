#!/usr/bin/env bash
# End-to-end local demo:
#   1. build a fake ECR-tagged container and load it into kind
#   2. send a synthetic ECR push event to SQS
#   3. wait for the image-updater to patch the Application
#   4. sync the demo pod to the Application's ECR image ref
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
# shellcheck source=dev/common.sh
source "${ROOT_DIR}/dev/common.sh"

./dev/build-test-image.sh

IMAGE_TAG="$(cat dev/.last-image-tag)"
IMAGE_DIGEST="$(cat dev/.last-image-digest)"
FULL_IMAGE="$(ecr_image_ref "${IMAGE_TAG}")"

echo "==> Sending synthetic ECR event for ${FULL_IMAGE}"
IMAGE_TAG="$IMAGE_TAG" IMAGE_DIGEST="$IMAGE_DIGEST" ./dev/send-test-event.sh

echo "==> Waiting for Application spec to reference ${FULL_IMAGE}"
CURRENT=""
for _ in $(seq 1 60); do
  CURRENT="$(kubectl -n argocd get application demo-app -o jsonpath='{.spec.source.kustomize.images[0]}' 2>/dev/null || true)"
  if [[ "$CURRENT" == *"${IMAGE_TAG}"* ]]; then
    echo "Application updated: ${CURRENT}"
    break
  fi
  sleep 2
done

if [[ "$CURRENT" != *"${IMAGE_TAG}"* ]]; then
  echo "Timed out waiting for Application update. Rebuild/restart the controller if you changed Go code recently:" >&2
  echo "  make docker-build IMG=argocd-image-updater-controller:dev" >&2
  echo "  kind load docker-image argocd-image-updater-controller:dev --name image-updater-dev" >&2
  echo "  kubectl -n argocd rollout restart deploy/argocd-image-updater-controller" >&2
  kubectl -n argocd logs deploy/argocd-image-updater-controller --tail=30 >&2 || true
  exit 1
fi

echo "==> Syncing demo pod"
./dev/sync-workload.sh

echo
echo "Pod version file:"
kubectl -n default exec deploy/demo-app -- cat /version.txt
