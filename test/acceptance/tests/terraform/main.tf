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
  consul_http_addr = "${var.tls ? "https" : "http"}://${module.dev_consul_server.server_dns}:${var.tls ? 8501 : 8500}"
}

data "aws_secretsmanager_secret_version" "ca_cert" {
  count     = var.tls ? 1 : 0
  secret_id = module.dev_consul_server.ca_cert_arn
}

data "aws_secretsmanager_secret_version" "bootstrap_token" {
  count     = var.acls ? 1 : 0
  secret_id = module.dev_consul_server.bootstrap_token_secret_arn
}

resource "aws_ssm_parameter" "acl-token" {
  count = var.acls ? 1 : 0
  name  = "/${local.name}/acl-token"
  type  = "SecureString"
  value = data.aws_secretsmanager_secret_version.bootstrap_token[0].secret_string
}

resource "aws_ssm_parameter" "ca-cert" {
  count = var.tls ? 1 : 0
  name  = "/${local.name}/ca-cert"
  type  = "SecureString"
  value = data.aws_secretsmanager_secret_version.ca_cert[0].secret_string
}

module "lambda-registration" {
  source = "../../../../modules/lambda-registrator"

  name                   = "lambda-registrator-${var.suffix}"
  consul_http_addr       = local.consul_http_addr
  consul_ca_cert_path    = var.tls ? aws_ssm_parameter.ca-cert[0].name : ""
  consul_http_token_path = var.acls ? aws_ssm_parameter.acl-token[0].name : ""
  ecr_image_uri          = var.ecr_image_uri
}
