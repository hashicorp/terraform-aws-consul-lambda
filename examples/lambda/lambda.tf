# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

locals {
  lambda_source_path      = "src/lambda"
  lambda_source_code_hash = filebase64sha256("${local.lambda_source_path}/main.go")
  lambda_function         = "${local.lambda_source_path}/function.zip"
}

// Deploy the Consul Lambda extension as a Lambda layer.
resource "aws_lambda_layer_version" "consul_lambda_extension" {
  layer_name       = "consul-lambda-extension"
  filename         = "consul-lambda-extension.zip"
  source_code_hash = filebase64sha256("consul-lambda-extension.zip")
  description      = "Consul service mesh extension for AWS Lambda"
}

// Lambda function that is called by an ECS task in the Consul service mesh.
module "lambda_app_1" {
  source    = "./lambda"
  name      = "${var.name}-lambda-app-1"
  file_path = local.lambda_function
  role_arn  = aws_iam_role.lambda.arn

  env = {
    LOG_LEVEL = "debug"
  }
}

// Lambda function that calls an ECS task in the Consul service mesh.
module "lambda_app_2" {
  source    = "./lambda"
  name      = "${var.name}-lambda-app-2"
  file_path = local.lambda_function
  role_arn  = aws_iam_role.lambda.arn

  // Set the upstreams environment variable on the Lambda function.
  // This variable is an implementation detail of the Lambda function (not the Consul integration)
  // and controls the upstreams the function calls via the loopback address.
  // Calls to these upstreams on the loopback will get proxied through the mesh gateway via the
  // Consul-Lambda extension.
  env = {
    UPSTREAMS     = "http://localhost:1234"
    TRACE_ENABLED = "true"
    LOG_LEVEL     = "debug"
  }

  // Consul-Lambda extension configuration.
  consul_lambda_extension_arn  = aws_lambda_layer_version.consul_lambda_extension.arn
  consul_extension_data_prefix = "/${var.name}"
  consul_mesh_gateway_uri      = "${module.mesh_gateway.wan_address}:${module.mesh_gateway.wan_port}"
  consul_upstreams             = ["${var.name}-ecs-app-2:1234"]
}

resource "aws_iam_policy" "lambda" {
  name        = "${var.name}-lambda-policy"
  path        = "/"
  description = "IAM policy for Lambda functions"

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
      "Resource": "arn:aws:ssm:*:*:parameter/${var.name}/*",
      "Effect": "Allow"
    }
  ]
}
EOF
}

resource "aws_iam_role" "lambda" {
  name = "${var.name}-lambda-role"

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

resource "aws_iam_role_policy_attachment" "lambda" {
  role       = aws_iam_role.lambda.name
  policy_arn = aws_iam_policy.lambda.arn
}
