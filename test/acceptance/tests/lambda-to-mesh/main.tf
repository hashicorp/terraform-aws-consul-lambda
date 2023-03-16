# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "4.14.0"
    }
  }
}

provider "aws" {
  alias  = "provider"
  region = var.region
}

module "lambda_to_mesh" {
  source = "../lambda"
  providers = {
    aws.provider = aws.provider
  }
  name   = var.name
  tags   = var.tags
  region = var.region
  env    = var.env
  layers = var.layers
}

