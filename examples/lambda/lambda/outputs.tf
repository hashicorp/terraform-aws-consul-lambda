# Copyright IBM Corp. 2022, 2025
# SPDX-License-Identifier: MPL-2.0

output "arn" {
  description = "The ARN of the Lambda function."
  value       = aws_lambda_function.lambda_app.arn
}
