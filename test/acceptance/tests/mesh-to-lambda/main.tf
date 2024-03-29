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

module "mesh_to_lambda" {
  source = "../lambda"
  providers = {
    aws.provider = aws.provider
  }
  name   = var.name
  tags   = var.tags
  region = var.region
  arch   = var.arch
}
