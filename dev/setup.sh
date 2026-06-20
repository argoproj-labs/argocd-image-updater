#!/usr/bin/env bash
# Local dev stack: kind + LocalStack (SQS/EventBridge/STS).
# Note: LocalStack Community may not emulate ECR; use dev/send-test-event.sh to
# inject synthetic push events into SQS when ECR is unavailable.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
# shellcheck source=dev/common.sh
source "${ROOT_DIR}/dev/common.sh"

CLUSTER_NAME="${CLUSTER_NAME:-image-updater-dev}"
AWS_REGION="${AWS_REGION:-us-east-1}"
AWS_ENDPOINT_HOST="${AWS_ENDPOINT_HOST:-http://localhost:4566}"
AWS_ENDPOINT_K8S="${AWS_ENDPOINT_K8S:-http://host.docker.internal:4566}"
REPO_NAME="${REPO_NAME:-demo-app}"
QUEUE_NAME="${QUEUE_NAME:-ecr-push-events}"
RULE_NAME="${RULE_NAME:-ecr-image-push}"
CONTROLLER_IMAGE="${CONTROLLER_IMAGE:-argocd-image-updater-controller:dev}"
NAMESPACE="${NAMESPACE:-argocd}"

export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION="$AWS_REGION"

awslocal() {
  aws --endpoint-url "$AWS_ENDPOINT_HOST" "$@"
}

echo "==> Starting LocalStack"
docker compose -f dev/docker-compose.yaml up -d
for _ in $(seq 1 30); do
  if curl -sf "${AWS_ENDPOINT_HOST}/_localstack/health" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

echo "==> Creating kind cluster ${CLUSTER_NAME}"
if ! kind get clusters | grep -qx "$CLUSTER_NAME"; then
  kind create cluster --name "$CLUSTER_NAME"
fi

echo "==> Building controller image"
make docker-build IMG="$CONTROLLER_IMAGE"
kind load docker-image "$CONTROLLER_IMAGE" --name "$CLUSTER_NAME"

echo "==> Installing ImageUpdater CRD and RBAC"
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f config/crd/bases/argocd-image-updater.argoproj.io_imageupdaters.yaml
kubectl -n "$NAMESPACE" apply -f config/rbac/service_account.yaml
kubectl -n "$NAMESPACE" apply -f config/rbac/role.yaml
kubectl -n "$NAMESPACE" apply -f config/rbac/role_binding.yaml
kubectl -n "$NAMESPACE" apply -f config/rbac/leader_election_role.yaml
kubectl -n "$NAMESPACE" apply -f config/rbac/leader_election_role_binding.yaml

echo "==> Creating LocalStack ECR repository"
awslocal ecr create-repository --repository-name "$REPO_NAME" >/dev/null 2>&1 || true

ACCOUNT_ID="$(awslocal sts get-caller-identity --query Account --output text)"
QUEUE_URL="$(awslocal sqs create-queue \
  --queue-name "$QUEUE_NAME" \
  --attributes MessageRetentionPeriod=3600 \
  --query QueueUrl --output text)"
QUEUE_ARN="arn:aws:sqs:${AWS_REGION}:${ACCOUNT_ID}:${QUEUE_NAME}"

echo "==> Wiring EventBridge rule to SQS"
awslocal events put-rule \
  --name "$RULE_NAME" \
  --event-pattern '{"source":["aws.ecr"],"detail-type":["ECR Image Action"],"detail":{"action-type":["PUSH"],"result":["SUCCESS"]}}' >/dev/null

awslocal sqs set-queue-attributes \
  --queue-url "$QUEUE_URL" \
  --attributes "{\"Policy\":\"{\\\"Version\\\":\\\"2012-10-17\\\",\\\"Statement\\\":[{\\\"Effect\\\":\\\"Allow\\\",\\\"Principal\\\":{\\\"Service\\\":\\\"events.amazonaws.com\\\"},\\\"Action\\\":\\\"sqs:SendMessage\\\",\\\"Resource\\\":\\\"${QUEUE_ARN}\\\"}]}\"}"

awslocal events put-targets \
  --rule "$RULE_NAME" \
  --targets "Id=1,Arn=${QUEUE_ARN}" >/dev/null

echo "==> Deploying controller with SQS poller enabled"
kubectl -n "$NAMESPACE" apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: argocd-image-updater-controller
  labels:
    app.kubernetes.io/name: argocd-image-updater
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: argocd-image-updater
  template:
    metadata:
      labels:
        app.kubernetes.io/name: argocd-image-updater
    spec:
      serviceAccountName: argocd-image-updater-controller
      containers:
      - name: controller
        image: ${CONTROLLER_IMAGE}
        imagePullPolicy: IfNotPresent
        args:
          - run
          - --interval=0
          - --warmup-cache=false
          - --health-probe-bind-address=0
          - --metrics-bind-address=0
          - --leader-election=false
          - --enable-sqs=true
          - --sqs-queue-url=${QUEUE_URL}
          - --aws-region=${AWS_REGION}
          - --aws-endpoint-url=${AWS_ENDPOINT_K8S}
          - --ecr-fallback-on-describe-error=true
          - --loglevel=debug
        env:
          - name: AWS_ACCESS_KEY_ID
            value: test
          - name: AWS_SECRET_ACCESS_KEY
            value: test
          - name: AWS_DEFAULT_REGION
            value: ${AWS_REGION}
EOF

echo "==> Installing Argo CD CRDs (Application + AppProject only)"
ARGO_CD_VERSION="${ARGO_CD_VERSION:-v3.4.3}"
CRD_BASE="https://raw.githubusercontent.com/argoproj/argo-cd/${ARGO_CD_VERSION}/manifests/crds"
# Install only the CRDs needed for local dev. The full crds kustomization also
# applies ApplicationSet, whose OpenAPI schema is too large for client-side apply
# (metadata.annotations last-applied-configuration exceeds the 256KiB limit).
kubectl apply --server-side --force-conflicts -f "${CRD_BASE}/application-crd.yaml"
kubectl apply --server-side --force-conflicts -f "${CRD_BASE}/appproject-crd.yaml"

echo "==> Applying demo AppProject, Application, and ImageUpdater"
kubectl -n "$NAMESPACE" apply -f dev/manifests/argocd-project.yaml
kubectl -n "$NAMESPACE" apply -f dev/manifests/application.yaml
kubectl -n "$NAMESPACE" apply -f dev/manifests/imageupdater.yaml
kubectl apply -f dev/manifests/workload.yaml

echo "==> Building initial fake ECR image $(ecr_image_ref dev-initial)"
IMAGE_TAG=dev-initial ./dev/build-test-image.sh >/dev/null

cat <<INFO

Local dev environment is ready.

  ECR registry : ${ECR_REGISTRY}
  Repository   : ${ECR_REPO}
  SQS queue    : ${QUEUE_URL}

Check cluster state:
  dev/show-state.sh

Send a synthetic ECR push event:
  dev/send-test-event.sh

Watch the controller apply the update to the Application spec:
  kubectl -n argocd get application demo-app -o jsonpath='{.spec.source.kustomize.images}{"\n"}' -w

End-to-end demo (build fake ECR image, send event, sync pod):
  dev/demo-update.sh

Priority integration tests (stale events, filters, re-push):
  dev/test-priority.sh

INFO
