#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

AWS_REGION="${AWS_REGION:-us-east-1}"
AWS_ENDPOINT="${AWS_ENDPOINT:-http://localhost:4566}"
REPO_NAME="${REPO_NAME:-demo-app}"
IMAGE_TAG="${IMAGE_TAG:-dev-$(date +%s)}"

export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION="$AWS_REGION"

awslocal() {
  aws --endpoint-url "$AWS_ENDPOINT" "$@"
}

ACCOUNT_ID="$(awslocal sts get-caller-identity --query Account --output text)"
REGISTRY="${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"
IMAGE_URI="${REGISTRY}/${REPO_NAME}:${IMAGE_TAG}"

echo "==> Logging in to LocalStack ECR"
awslocal ecr get-login-password | docker login --username AWS --password-stdin "$REGISTRY"

echo "==> Building and pushing ${IMAGE_URI}"
docker build -t "$IMAGE_URI" -f - . <<'DOCKERFILE'
FROM alpine:3.20
RUN echo "demo" > /demo.txt
DOCKERFILE

docker push "$IMAGE_URI"

echo "==> Waiting for SQS message"
QUEUE_URL="$(awslocal sqs get-queue-url --queue-name ecr-push-events --query QueueUrl --output text)"
for _ in $(seq 1 30); do
  COUNT="$(awslocal sqs get-queue-attributes --queue-url "$QUEUE_URL" --attribute-names ApproximateNumberOfMessages --query 'Attributes.ApproximateNumberOfMessages' --output text)"
  if [[ "$COUNT" != "0" ]]; then
    echo "SQS received ${COUNT} message(s) for tag ${IMAGE_TAG}"
    exit 0
  fi
  sleep 2
done

echo "Timed out waiting for ECR push event in SQS" >&2
exit 1
