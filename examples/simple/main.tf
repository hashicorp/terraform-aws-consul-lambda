terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "3.63.0"
    }
  }
}

provider "aws" {
  region = var.region
}

data "aws_caller_identity" "current" {}

data "aws_availability_zones" "available" {
  filter {
    name   = "opt-in-status"
    values = ["opt-in-not-required"]
  }
}

locals {
  name             = "lambda-registrator-${var.suffix}"
  short_name       = "lr-${var.suffix}"
  consul_http_addr = "${var.tls ? "https" : "http"}://${module.dev_consul_server.lb_dns_name}:${var.tls ? 8501 : 8500}"
}

resource "aws_cloudwatch_log_group" "log_group" {
  name = local.name
}

data "aws_secretsmanager_secret_version" "ca_cert" {
  secret_id = module.dev_consul_server.ca_cert_arn
}

data "aws_secretsmanager_secret_version" "bootstrap_token" {
  secret_id = module.dev_consul_server.bootstrap_token_secret_arn
}

resource "aws_ssm_parameter" "acl-token" {
  count = var.acls ? 1 : 0
  name  = "/lambda-registrator/acl-token"
  type  = "SecureString"
  value = "10f73c0c-bb8a-6f7b-3da6-772ccc1c121e"
}

resource "aws_ssm_parameter" "ca-cert" {
  count = var.tls ? 1 : 0
  name  = "/lambda-registrator/ca-cert"
  type  = "SecureString"
  value = data.aws_secretsmanager_secret_version.ca_cert.secret_string
}

module "lambda-registration" {
  source = "../../modules/lambda-registrator"

  name                   = "lambda-registrator-${var.suffix}"
  consul_http_addr       = local.consul_http_addr
  consul_ca_cert_path    = var.tls ? aws_ssm_parameter.ca-cert[0].name : ""
  consul_http_token_path = var.acls ? aws_ssm_parameter.acl-token[0].name : ""
  ecr_image_uri          = "${aws_ecr_repository.lambda-registrator.repository_url}@${data.aws_ecr_image.lambda-registrator.id}"
}
