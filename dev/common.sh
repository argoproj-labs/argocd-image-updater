# Shared LocalStack / fake ECR settings for dev scripts.
ECR_ACCOUNT_ID="${ECR_ACCOUNT_ID:-000000000000}"
ECR_REGION="${ECR_REGION:-us-east-1}"
ECR_REPO="${ECR_REPO:-demo-app}"
ECR_REGISTRY="${ECR_REGISTRY:-${ECR_ACCOUNT_ID}.dkr.ecr.${ECR_REGION}.amazonaws.com}"
ECR_IMAGE="${ECR_REGISTRY}/${ECR_REPO}"

ecr_image_ref() {
  local tag="$1"
  echo "${ECR_IMAGE}:${tag}"
}
