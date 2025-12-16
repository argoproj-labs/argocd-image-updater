terraform {
  required_version = ">= 1.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

# EventBridge Rule to capture ECR push events
resource "aws_cloudwatch_event_rule" "ecr_push" {
  name        = "argocd-image-updater-ecr-push"
  description = "Capture ECR image push events for ArgoCD Image Updater"

  event_pattern = jsonencode({
    source      = ["aws.ecr"]
    detail-type = ["ECR Image Action"]
    detail = {
      action-type = ["PUSH"]
      result      = ["SUCCESS"]
      # Filter for events with image tags (excludes untagged/manifest-only pushes)
      image-tag = [{
        exists = true
      }]
      # Optional: Filter by specific repositories
      repository-name = length(var.ecr_repository_filter) > 0 ? var.ecr_repository_filter : null
    }
  })

  tags = var.tags
}

# EventBridge Connection for API authentication
resource "aws_cloudwatch_event_connection" "webhook" {
  name               = "argocd-image-updater-webhook"
  description        = "Connection to ArgoCD Image Updater webhook"
  authorization_type = "API_KEY"

  auth_parameters {
    api_key {
      key   = "X-Webhook-Secret"
      value = var.webhook_secret
    }
  }
}

# API Destination pointing to ArgoCD Image Updater webhook
resource "aws_cloudwatch_event_api_destination" "webhook" {
  name                             = "argocd-image-updater-webhook"
  description                      = "ArgoCD Image Updater CloudEvents webhook endpoint"
  invocation_endpoint              = var.webhook_url
  http_method                      = "POST"
  invocation_rate_limit_per_second = 10
  connection_arn                   = aws_cloudwatch_event_connection.webhook.arn
}

# IAM Role for EventBridge to invoke API Destination
resource "aws_iam_role" "eventbridge" {
  name               = "argocd-image-updater-eventbridge-role"
  assume_role_policy = data.aws_iam_policy_document.eventbridge_assume_role.json
  tags               = var.tags
}

data "aws_iam_policy_document" "eventbridge_assume_role" {
  statement {
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["events.amazonaws.com"]
    }

    actions = ["sts:AssumeRole"]
  }
}

# IAM Policy for EventBridge to invoke API Destination
resource "aws_iam_role_policy" "eventbridge_invoke_api_destination" {
  name   = "invoke-api-destination"
  role   = aws_iam_role.eventbridge.id
  policy = data.aws_iam_policy_document.eventbridge_invoke_api_destination.json
}

data "aws_iam_policy_document" "eventbridge_invoke_api_destination" {
  statement {
    effect = "Allow"

    actions = [
      "events:InvokeApiDestination"
    ]

    resources = [
      aws_cloudwatch_event_api_destination.webhook.arn
    ]
  }
}

# EventBridge Target with Input Transformer (ECR -> CloudEvents)
resource "aws_cloudwatch_event_target" "api_destination" {
  rule      = aws_cloudwatch_event_rule.ecr_push.name
  target_id = "ArgocdImageUpdaterCloudEvent"
  arn       = aws_cloudwatch_event_api_destination.webhook.arn
  role_arn  = aws_iam_role.eventbridge.arn

  input_transformer {
    input_paths = {
      id      = "$.id"
      time    = "$.time"
      account = "$.account"
      region  = "$.region"
      repo    = "$.detail.repository-name"
      digest  = "$.detail.image-digest"
      tag     = "$.detail.image-tag"
    }

    input_template = <<-EOF
    {
      "specversion": "1.0",
      "id": "<id>",
      "type": "com.amazon.ecr.image.push",
      "source": "urn:aws:ecr:<region>:<account>:repository/<repo>",
      "subject": "<repo>:<tag>",
      "time": "<time>",
      "datacontenttype": "application/json",
      "data": {
        "repositoryName": "<repo>",
        "imageDigest": "<digest>",
        "imageTag": "<tag>",
        "registryId": "<account>"
      }
    }
    EOF
  }

  retry_policy {
    maximum_event_age_in_seconds = 3600
    maximum_retry_attempts       = 3
  }
}
