#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=dev/common.sh
source "${ROOT_DIR}/dev/common.sh"

CLUSTER_NAME="${CLUSTER_NAME:-image-updater-dev}"
IMAGE_TAG="${IMAGE_TAG:-dev-$(date +%s)}"
BUILD_BUST="${BUILD_BUST:-$(date +%s)}"
FULL_IMAGE="$(ecr_image_ref "${IMAGE_TAG}")"

echo "==> Building fake ECR image ${FULL_IMAGE}"
docker build \
  --build-arg "BUILD_TAG=${IMAGE_TAG}" \
  --build-arg "BUILD_BUST=${BUILD_BUST}" \
  -t "${FULL_IMAGE}" \
  -f "${ROOT_DIR}/dev/test-image/Dockerfile" \
  "${ROOT_DIR}/dev/test-image"

DIGEST="$(docker inspect -f '{{.Id}}' "${FULL_IMAGE}")"

echo "==> Loading ${FULL_IMAGE} into kind cluster ${CLUSTER_NAME}"
kind load docker-image "${FULL_IMAGE}" --name "${CLUSTER_NAME}"

echo "${IMAGE_TAG}" > "${ROOT_DIR}/dev/.last-image-tag"
echo "${DIGEST}" > "${ROOT_DIR}/dev/.last-image-digest"
echo "${FULL_IMAGE}" > "${ROOT_DIR}/dev/.last-image-ref"

echo
echo "Built and loaded:"
echo "  ecr image   : ${FULL_IMAGE}"
echo "  digest      : ${DIGEST}"
echo
echo "Next: IMAGE_TAG=${IMAGE_TAG} IMAGE_DIGEST=${DIGEST} dev/send-test-event.sh"
