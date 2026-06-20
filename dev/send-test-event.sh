#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=dev/common.sh
source "${ROOT_DIR}/dev/common.sh"

AWS_ENDPOINT="${AWS_ENDPOINT:-http://localhost:4566}"
QUEUE_NAME="${QUEUE_NAME:-ecr-push-events}"
IMAGE_TAG="${IMAGE_TAG:-dev-$(date +%s)}"
IMAGE_DIGEST="${IMAGE_DIGEST:-sha256:deadbeef}"
REPOSITORY="${REPOSITORY:-${ECR_REPO}}"
EVENT_TIME="${EVENT_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION="$ECR_REGION"

awslocal() {
  aws --endpoint-url "$AWS_ENDPOINT" "$@"
}

QUEUE_URL="$(awslocal sqs get-queue-url --queue-name "$QUEUE_NAME" --query QueueUrl --output text 2>/dev/null || true)"
if [[ -z "$QUEUE_URL" ]]; then
  QUEUE_URL="$(awslocal sqs create-queue \
    --queue-name "$QUEUE_NAME" \
    --attributes MessageRetentionPeriod=3600 \
    --query QueueUrl --output text)"
fi

BODY="$(cat <<JSON
{
  "version": "0",
  "id": "smoke-test",
  "detail-type": "ECR Image Action",
  "source": "aws.ecr",
  "account": "${ECR_ACCOUNT_ID}",
  "time": "${EVENT_TIME}",
  "region": "${ECR_REGION}",
  "detail": {
    "result": "SUCCESS",
    "repository-name": "${REPOSITORY}",
    "image-digest": "${IMAGE_DIGEST}",
    "action-type": "PUSH",
    "image-tag": "${IMAGE_TAG}"
  }
}
JSON
)"

awslocal sqs send-message --queue-url "$QUEUE_URL" --message-body "$BODY" >/dev/null
echo "Sent synthetic ECR push event for $(ecr_image_ref "${IMAGE_TAG}") to ${QUEUE_URL}"
