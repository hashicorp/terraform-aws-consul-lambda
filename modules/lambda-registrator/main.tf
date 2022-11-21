# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

locals {
  on_vpc = length(var.subnet_ids) > 0 && length(var.security_group_ids) > 0
  vpc_config = local.on_vpc ? [{
    subnet_ids         = var.subnet_ids
    security_group_ids = var.security_group_ids
  }] : []
  cron_key          = "${var.name}-cron"
  lambda_events_key = "${var.name}-lambda_events"
  image_tag     = split(":", var.consul_lambda_registrator_image)[1]
  ecr_image_uri = "${data.aws_caller_identity.current.account_id}.dkr.ecr.${var.region}.amazonaws.com/${var.private_repo_name}:${local.image_tag}"
  //See comment in line 157 for explanation
//  ecr_image_uri_pull-through = "${data.aws_caller_identity.current.account_id}.dkr.ecr.${var.region}.amazonaws.com/ecr-public/hashicorp/${var.private_repo_name}:${local.image_tag}"
}
data "aws_caller_identity" "current" {}

resource "aws_iam_role" "registration" {
  name = var.name

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

resource "aws_iam_policy" "policy" {
  name        = "${var.name}-policy"
  path        = "/"
  description = "IAM policy for consul registration service"

  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
%{if var.consul_ca_cert_path != ""~}
    {
      "Effect": "Allow",
      "Action": [
        "ssm:GetParameter"
      ],
      "Resource": "arn:aws:ssm:*:*:parameter${var.consul_ca_cert_path}"
    },
    {
      "Effect": "Allow",
      "Action": [
        "kms:Decrypt"
      ],
      "Resource": "arn:aws:ssm:*:*:parameter${var.consul_ca_cert_path}"
    },
%{endif~}
%{if var.consul_http_token_path != ""~}
    {
      "Effect": "Allow",
      "Action": [
        "ssm:GetParameter"
      ],
      "Resource": "arn:aws:ssm:*:*:parameter${var.consul_http_token_path}"
    },
    {
      "Effect": "Allow",
      "Action": [
        "kms:Decrypt"
      ],
      "Resource": "arn:aws:ssm:*:*:parameter${var.consul_http_token_path}"
    },
%{endif~}
%{if local.on_vpc~}
    {
      "Action": [
        "ec2:CreateNetworkInterface",
        "ec2:DescribeNetworkInterfaces",
        "ec2:DeleteNetworkInterface"

      ],
      "Resource": "*",
      "Effect": "Allow"
    },
%{endif~}
%{if var.consul_extension_data_prefix != ""~}
    {
      "Effect": "Allow",
      "Action": [
        "ssm:PutParameter",
        "ssm:DeleteParameter"
      ],
      "Resource": "arn:aws:ssm:*:*:parameter${var.consul_extension_data_prefix}/*"
    },
%{endif~}
    {
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": "*",
      "Effect": "Allow"
    },
    {
      "Action": [
        "lambda:GetFunction"
      ],
      "Resource": "arn:aws:lambda:*",
      "Effect": "Allow"
    },
    {
      "Action": [
        "lambda:ListFunctions"
      ],
      "Resource": "*",
      "Effect": "Allow"
    }
  ]
}
EOF
}

resource "aws_iam_role_policy_attachment" "lambda_logs" {
  role       = aws_iam_role.registration.name
  policy_arn = aws_iam_policy.policy.arn
}

resource "aws_ecr_repository" "lambda-registrator" {
  name = var.private_repo_name
  force_delete = true
}

resource "null_resource" "pull_and_republish_ecr_image" {
  count = var.pull_through ? 0 : 1
  triggers = {
    always_run = timestamp()
  }

  provisioner "local-exec" {
    command = <<EOF
    aws ecr get-login-password --region ${var.region} | docker login --username AWS --password-stdin ${data.aws_caller_identity.current.account_id}.dkr.ecr.${var.region}.amazonaws.com
    docker pull ${var.consul_lambda_registrator_image}
    docker tag ${var.consul_lambda_registrator_image}  ${local.ecr_image_uri}
    docker push ${local.ecr_image_uri}
    EOF
  }

  depends_on = [
    aws_ecr_repository.lambda-registrator
  ]
}

// Was able to get the pull through cache running via command line using the same aws-docker login as in line 145 and doing a modified pull using ecr pull-through variable in locals above
// through the command line - but unable to replicate that behavior here. Paul was paired with me when we made this breakthrough so may be valuable to ping him for clearer explanation.
// Commenting out below resource for now.
#resource "null_resource" "ecr_pull_through_cache" {
#  count = var.pull_through ? 1 : 0
#  triggers = {
#    always_run = timestamp()
#  }
#
#  provisioner "local-exec" {
#    command = <<EOF
#    aws ecr create-pull-through-cache-rule \
#     --ecr-repository-prefix ecr-public \
#     --upstream-registry-url public.ecr.aws \
#     --region ${var.region}
#    EOF
#  }
  //Need to find a way to force delete the created pull through rule, otherwise error will be thrown if this is run multiple times sans delete
  //since pull through rule already exists from previous apply
#}

resource "aws_lambda_function" "registration" {
  image_uri                      = local.ecr_image_uri
  package_type                   = "Image"
  function_name                  = var.name
  role                           = aws_iam_role.registration.arn
  timeout                        = var.timeout
  reserved_concurrent_executions = var.reserved_concurrent_executions
  layers                         = []
  tags                           = var.tags
  environment {
    variables = merge(
      {
        CONSUL_HTTP_ADDR = var.consul_http_addr,
        NODE_NAME        = var.node_name,
        ENTERPRISE       = var.enterprise,
      },
      length(var.partitions) > 0 ? {
        PARTITIONS = join(",", var.partitions),
      } : {},
      var.consul_http_token_path != "" ? {
        CONSUL_HTTP_TOKEN_PATH = var.consul_http_token_path
      } : {},
      var.consul_ca_cert_path != "" ? {
        CONSUL_CACERT_PATH = var.consul_ca_cert_path
        CONSUL_HTTP_SSL    = "true"
      } : {},
      var.consul_extension_data_prefix != "" ? {
        CONSUL_EXTENSION_DATA_PREFIX = var.consul_extension_data_prefix
      } : {},
      var.consul_extension_data_tier != "" ? {
        CONSUL_EXTENSION_DATA_TIER = var.consul_extension_data_tier
      } : {}
    )
  }
  dynamic "vpc_config" {
    for_each = local.vpc_config
    content {
      subnet_ids         = vpc_config.value["subnet_ids"]
      security_group_ids = vpc_config.value["security_group_ids"]
    }
  }

  depends_on = [
    null_resource.pull_and_republish_ecr_image
  ]
  //Once pull through is properly configured, need to add null_resource.ecr_pull_through_cache into depends_on array
}

module "eventbridge" {
  source  = "terraform-aws-modules/eventbridge/aws"
  version = "1.17.3"

  create_bus = false
  role_name  = "${var.name}-eventbridge"

  rules = {
    "${local.lambda_events_key}" = {
      description = "Capture Lambda events from CloudTrail"
      enabled     = true
      event_pattern = jsonencode({
        "source" : ["aws.lambda"],
        "detail-type" : ["AWS API Call via CloudTrail"],
        "detail" : {
          "eventSource" : ["lambda.amazonaws.com"]
          "eventName" : [
            "CreateFunction20150331",
            "CreateFunction",
            "TagResource20170331v2",
            "TagResource20170331",
            "TagResource",
            "UntagResource20170331v2",
            "UntagResource20170331",
            "UntagResource",
          ]
        }
      })
    }
    "${local.cron_key}" = {
      description         = "Periodically trigger the Lambda"
      schedule_expression = "rate(${var.sync_frequency_in_minutes} ${var.sync_frequency_in_minutes > 1 ? "minutes" : "minute"})"
    }
  }

  targets = {
    "${local.lambda_events_key}" = [
      {
        name = "Process CloudTrail events"
        arn  = aws_lambda_function.registration.arn
      },
    ]
    "${local.cron_key}" = [
      {
        name = "Periodic sync"
        arn  = aws_lambda_function.registration.arn
      },
    ]
  }
}

resource "aws_lambda_permission" "cloudtrail_invoke" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.registration.function_name
  principal     = "events.amazonaws.com"
  source_arn    = module.eventbridge.eventbridge_rule_arns[local.lambda_events_key]
}

resource "aws_lambda_permission" "cron_invoke" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.registration.function_name
  principal     = "events.amazonaws.com"
  source_arn    = module.eventbridge.eventbridge_rule_arns[local.cron_key]
}
