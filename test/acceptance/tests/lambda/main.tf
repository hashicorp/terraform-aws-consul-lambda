# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "4.67.0"

      configuration_aliases = [aws.provider]
    }
  }
}

locals {
  example_path = "${path.module}/example.zip"
  env          = length(var.env) > 0 ? [{ "variables" = var.env }] : []
}

resource "aws_lambda_function" "example" {
  filename      = local.example_path
  function_name = var.name
  role          = aws_iam_role.example.arn
  handler       = "main"
  runtime       = "provided.al2"
  tags          = merge(var.tags, { time = timestamp() })
  layers        = var.layers
  architectures = var.arch == "arm64" ? ["arm64"] : ["x86_64"]
  dynamic "environment" {
    for_each = local.env
    content {
      variables = environment.value.variables
    }
  }
}

resource "aws_lambda_alias" "example-prod" {
  name             = "prod"
  function_name    = aws_lambda_function.example.arn
  function_version = aws_lambda_function.example.version
}

resource "aws_lambda_alias" "example-dev" {
  name             = "dev"
  function_name    = aws_lambda_function.example.arn
  function_version = aws_lambda_function.example.version
}

resource "aws_iam_policy" "lambda_logging" {
  name        = "${var.name}-policy"
  path        = "/"
  description = "IAM policy for logging from a lambda"

  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": "arn:aws:logs:*:*:*",
      "Effect": "Allow"
    },
    {
      "Action": [
        "ssm:GetParameter"
      ],
      "Resource": "arn:aws:ssm:*:*:parameter/*",
      "Effect": "Allow"
    }
  ]
}
EOF
}

resource "aws_iam_role" "example" {
  name = "${var.name}-role"

  assume_role_policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Action": "sts:AssumeRole",
      "Principal": {
        "Service": "lambda.amazonaws.com"
      },
      "Effect": "Allow",
      "Sid": ""
    }
  ]
}
EOF
}

resource "aws_iam_role_policy_attachment" "lambda_logs" {
  role       = aws_iam_role.example.name
  policy_arn = aws_iam_policy.lambda_logging.arn
}
