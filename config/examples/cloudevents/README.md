# CloudEvents Webhook Example

Example Terraform configuration for setting up AWS EventBridge to send ECR push events to ArgoCD Image Updater via CloudEvents.

## Quick Start

### 1. Configure EventBridge with Terraform

```bash
cd terraform

# Set your variables
export TF_VAR_webhook_url="https://your-domain.com/webhook?type=cloudevents"
export TF_VAR_webhook_secret="your-webhook-secret"
export TF_VAR_aws_region="us-east-1"

# Apply the configuration
terraform init
terraform apply

# Return to parent directory
cd ..
```

### 2. Test the Webhook

```bash
./test-webhook.sh https://your-webhook-url/webhook?type=cloudevents your-secret
```

## Files

- `terraform/` - EventBridge configuration with input transformer for ECR events
- `test-webhook.sh` - Script to test the webhook endpoint

## Documentation

For complete setup instructions, see the [webhook documentation](../../../docs/configuration/webhook.md#aws-ecr-via-eventbridge-cloudevents).
