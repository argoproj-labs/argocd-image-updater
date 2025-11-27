output "eventbridge_rule_arn" {
  description = "ARN of the EventBridge rule"
  value       = aws_cloudwatch_event_rule.ecr_push.arn
}

output "api_destination_arn" {
  description = "ARN of the API destination"
  value       = aws_cloudwatch_event_api_destination.webhook.arn
}

output "eventbridge_role_arn" {
  description = "ARN of the EventBridge IAM role"
  value       = aws_iam_role.eventbridge.arn
}

output "webhook_endpoint" {
  description = "Configured webhook endpoint URL"
  value       = var.webhook_url
}
