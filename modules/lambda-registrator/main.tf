locals {
  registration_path = "registration.zip"
}

data "archive_file" "lambda_registration_zip" {
  type = "zip"
  // TODO This only works in dev but that is good enough for now.
  source_file = "../../lambda-registrator/lambda-registrator"
  output_path = local.registration_path
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
    {
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents"
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
  filename         = local.registration_path
  source_code_hash = data.archive_file.lambda_registration_zip.output_base64sha256
  function_name    = var.name
  role             = aws_iam_role.registration.arn
  handler          = "lambda-registrator"
  runtime          = "go1.x"
  timeout          = var.timeout
  layers           = []
  tags             = {}
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
}

module "eventbridge" {
  source = "terraform-aws-modules/eventbridge/aws"

  create_bus = false

  rules = {
    lambda_events = {
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
  }

  targets = {
    lambda_events = [
      {
        name = "Process CloudTrail events"
        arn  = aws_lambda_function.registration.arn
      },
    ]
  }
}

resource "aws_lambda_permission" "cloudtrail-invoke" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.registration.function_name
  principal     = "events.amazonaws.com"
  source_arn    = module.eventbridge.eventbridge_rule_arns.lambda_events
}
