data "aws_caller_identity" "current" {}

data "aws_region" "current" {}

locals {
  on_vpc = length(var.subnet_ids) > 0 && length(var.security_group_ids) > 0
  vpc_config = local.on_vpc ? [{
    subnet_ids         = var.subnet_ids
    security_group_ids = var.security_group_ids
  }] : []
  cron_key              = "${var.name}-cron"
  lambda_events_key     = "${var.name}-lambda_events"
  image_path            = trimprefix(var.ecr_image_uri, "public.ecr.aws/")
  public_ecr_image_uri  = var.ecr_image_uri != local.image_path
  account_id            = data.aws_caller_identity.current.account_id
  ecr_repository_prefix = "consul-ecr-public"
  ecr_image_uri         = local.public_ecr_image_uri ? "${local.account_id}.dkr.ecr.${data.aws_region.current.name}.amazonaws.com/${local.ecr_repository_prefix}/${local.image_path}" : var.ecr_image_uri
}

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

resource "aws_lambda_function" "registration" {
  image_uri                      = local.ecr_image_uri
  package_type                   = "Image"
  function_name                  = var.name
  role                           = aws_iam_role.registration.arn
  timeout                        = var.timeout
  reserved_concurrent_executions = var.reserved_concurrent_executions
  layers                         = []
  tags                           = {}
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

  depends_on = [time_sleep.wait_for_pull_through_cache_rule]
}

module "eventbridge" {
  source = "terraform-aws-modules/eventbridge/aws"

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

resource "aws_lambda_permission" "cloudtrail-invoke" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.registration.function_name
  principal     = "events.amazonaws.com"
  source_arn    = module.eventbridge.eventbridge_rule_arns[local.lambda_events_key]
}

resource "aws_lambda_permission" "cron-invoke" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.registration.function_name
  principal     = "events.amazonaws.com"
  source_arn    = module.eventbridge.eventbridge_rule_arns[local.cron_key]
}


resource "aws_ecr_pull_through_cache_rule" "ecr" {
  count                 = local.public_ecr_image_uri ? 1 : 0
  ecr_repository_prefix = local.ecr_repository_prefix
  upstream_registry_url = "public.ecr.aws"
}

// It takes a few minutes for the pull through cache rule to work with Lambda.
resource "time_sleep" "wait_for_pull_through_cache_rule" {
  count           = local.public_ecr_image_uri ? 1 : 0
  depends_on      = [aws_ecr_pull_through_cache_rule.ecr]
  create_duration = "3m"
}
