variable "aws_region" {
  description = "AWS region for ECR and EventBridge"
  type        = string
  default     = "us-east-1"
}

variable "webhook_url" {
  description = "ArgoCD Image Updater webhook endpoint URL (must use HTTPS)"
  type        = string
  # Example: "https://image-updater-webhook.example.com/webhook?type=cloudevents"

  validation {
    condition     = can(regex("^https://", var.webhook_url))
    error_message = "webhook_url must use HTTPS protocol for secure credential transmission."
  }
}

variable "webhook_secret" {
  description = "Secret for webhook authentication"
  type        = string
  sensitive   = true
}

variable "ecr_repository_filter" {
  description = "List of ECR repository names to monitor (empty list = all repositories)"
  type        = list(string)
  default     = []
}

variable "tags" {
  description = "Tags to apply to all resources"
  type        = map(string)
  default = {
    ManagedBy = "Terraform"
    Purpose   = "ArgoCD-Image-Updater"
  }
}
