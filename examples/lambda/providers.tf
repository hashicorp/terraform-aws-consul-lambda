# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "4.25.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "3.4.3"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "4.0.3"
    }

    docker = {
      source  = "kreuzwerker/docker"
      version = "3.0.2"
    }
  }
}

provider "aws" {
  region = var.region
}

# Equivalent of aws ecr get-login
data "aws_ecr_authorization_token" "ecr_auth" {}

provider "docker" {
  host = "unix:///var/run/docker.sock" # Use the appropriate Docker socket for your system
  registry_auth {
    username = data.aws_ecr_authorization_token.ecr_auth.user_name
    password = data.aws_ecr_authorization_token.ecr_auth.password
    address  = "${data.aws_caller_identity.current.account_id}.dkr.ecr.${var.region}.amazonaws.com"
  }
}
