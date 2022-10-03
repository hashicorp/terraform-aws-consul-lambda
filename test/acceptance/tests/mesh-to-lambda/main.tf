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

module "mesh_to_lambda" {
  source = "../lambda"
  providers = {
    aws.provider = aws.provider
  }
  name   = var.name
  tags   = var.tags
  region = var.region
}
