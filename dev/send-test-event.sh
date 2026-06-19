#!/usr/bin/env bash
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
AWS_ENDPOINT="${AWS_ENDPOINT:-http://localhost:4566}"
QUEUE_NAME="${QUEUE_NAME:-ecr-push-events}"
REPO_NAME="${REPO_NAME:-demo-app}"
IMAGE_TAG="${IMAGE_TAG:-dev-$(date +%s)}"
ACCOUNT_ID="${ACCOUNT_ID:-000000000000}"

export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION="$AWS_REGION"

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
  "account": "${ACCOUNT_ID}",
  "time": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "region": "${AWS_REGION}",
  "detail": {
    "result": "SUCCESS",
    "repository-name": "${REPO_NAME}",
    "image-digest": "sha256:deadbeef",
    "action-type": "PUSH",
    "image-tag": "${IMAGE_TAG}"
  }
}
JSON
)"

awslocal sqs send-message --queue-url "$QUEUE_URL" --message-body "$BODY" >/dev/null
echo "Sent synthetic ECR push event to ${QUEUE_URL} (tag=${IMAGE_TAG})"
