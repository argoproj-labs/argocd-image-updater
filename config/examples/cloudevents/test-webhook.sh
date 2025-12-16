#!/bin/bash
# Test script for CloudEvents webhook
# Usage: 
#   1. Make the script executable: chmod +x test-webhook.sh
#      Then run: ./test-webhook.sh <webhook-url> <secret>
#   2. Or run directly: bash test-webhook.sh <webhook-url> <secret>

set -e

WEBHOOK_URL="${1:-http://localhost:8080/webhook?type=cloudevents}"
SECRET="${2:-test-secret}"

echo "Testing CloudEvents webhook..."
echo "URL: ${WEBHOOK_URL}"
echo ""

# Test 1: ECR event
echo "Test 1: AWS ECR push event"
curl -X POST "${WEBHOOK_URL}" \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Secret: ${SECRET}" \
  -d '{
    "specversion": "1.0",
    "id": "test-ecr-event-001",
    "type": "com.amazon.ecr.image.push",
    "source": "urn:aws:ecr:us-east-1:123456789012:repository/my-app",
    "subject": "my-app:v1.2.3",
    "time": "2025-11-27T10:00:00Z",
    "datacontenttype": "application/json",
    "data": {
      "repositoryName": "my-app",
      "imageDigest": "sha256:abcdef1234567890",
      "imageTag": "v1.2.3",
      "registryId": "123456789012"
    }
  }' \
  -w "\nHTTP Status: %{http_code}\n\n"

# Test 2: Generic container event
echo "Test 2: Generic container push event"
curl -X POST "${WEBHOOK_URL}" \
  -H "Content-Type: application/cloudevents+json" \
  -H "X-Webhook-Secret: ${SECRET}" \
  -d '{
    "specversion": "1.0",
    "id": "test-generic-event-001",
    "type": "container.image.push",
    "source": "https://registry.example.com",
    "subject": "myapp/backend:v2.0.0",
    "time": "2025-11-27T10:05:00Z",
    "datacontenttype": "application/json",
    "data": {
      "repository": "myapp/backend",
      "tag": "v2.0.0",
      "digest": "sha256:fedcba0987654321",
      "registryUrl": "registry.example.com"
    }
  }' \
  -w "\nHTTP Status: %{http_code}\n\n"

# Test 3: Missing secret (should fail)
echo "Test 3: Missing secret (expected to fail)"
(
  curl -X POST "${WEBHOOK_URL}" \
    -H "Content-Type: application/json" \
    -d '{
      "specversion": "1.0",
      "id": "test-fail-event-001",
      "type": "com.amazon.ecr.image.push",
      "source": "urn:aws:ecr:us-east-1:123456789012:repository/test",
      "subject": "test:latest",
      "data": {
        "repositoryName": "test",
        "imageTag": "latest",
        "registryId": "123456789012"
      }
    }' \
    -w "\nHTTP Status: %{http_code}\n\n"
) || echo "Expected failure"

# Test 4: Invalid event type (should fail)
echo "Test 4: Invalid event type (expected to fail)"
(
  curl -X POST "${WEBHOOK_URL}" \
    -H "Content-Type: application/json" \
    -H "X-Webhook-Secret: ${SECRET}" \
    -d '{
      "specversion": "1.0",
      "id": "test-invalid-event-001",
      "type": "com.example.database.updated",
      "source": "https://db.example.com",
      "data": {
        "table": "users"
      }
    }' \
    -w "\nHTTP Status: %{http_code}\n\n"
) || echo "Expected failure"

echo "Testing complete!"
