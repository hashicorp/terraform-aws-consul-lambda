# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "4.67.0"
    }
  }
}

provider "aws" {
  region = var.region
}

provider "aws" {
  alias  = "provider"
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
  consul_http_addr = "https://${module.dev_consul_server.server_dns}:8501"
  enterprise       = var.consul_partition != ""
}

data "aws_secretsmanager_secret_version" "ca_cert" {
  secret_id  = module.dev_consul_server.ca_cert_arn
  depends_on = [module.dev_consul_server]
}

data "aws_secretsmanager_secret_version" "bootstrap_token" {
  count      = var.secure ? 1 : 0
  secret_id  = module.dev_consul_server.bootstrap_token_secret_arn
  depends_on = [module.dev_consul_server]
}

resource "aws_ssm_parameter" "acl_token" {
  count = var.secure ? 1 : 0
  name  = "/${local.name}/acl-token"
  type  = "SecureString"
  value = data.aws_secretsmanager_secret_version.bootstrap_token[0].secret_string
}

resource "aws_ssm_parameter" "ca_cert" {
  name  = "/${local.name}/ca-cert"
  type  = "SecureString"
  value = data.aws_secretsmanager_secret_version.ca_cert.secret_string
}

module "lambda-registration" {
  source = "../../../../modules/lambda-registrator"

  name                            = "lambda-registrator-1-${var.suffix}"
  consul_http_addr                = local.consul_http_addr
  consul_ca_cert_path             = aws_ssm_parameter.ca_cert.name
  consul_http_token_path          = var.secure ? aws_ssm_parameter.acl_token[0].name : ""
  ecr_image_uri                   = var.ecr_image_uri
  subnet_ids                      = var.private_subnets
  security_group_ids              = [data.aws_security_group.vpc_default.id]
  sync_frequency_in_minutes       = 1
  partitions                      = local.enterprise ? ["default", var.consul_partition] : []
  enterprise                      = local.enterprise
  enable_auto_publish_ecr_image   = var.enable_auto_publish_ecr_image
  consul_extension_data_prefix    = "/${var.suffix}"
  private_ecr_repo_name           = var.private_ecr_repo_name
  arch                            = var.arch == "arm64" ? "arm64" : "x86_64"
  consul_lambda_registrator_image = var.consul_lambda_registrator_image
}
