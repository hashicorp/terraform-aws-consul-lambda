terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "4.13.0"
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

resource "random_shuffle" "azs" {
  input = data.aws_availability_zones.available.names
}

resource "random_string" "suffix" {
  length  = 8
  special = false
  upper   = false
}

locals {
  suffix = random_string.suffix.result
  name   = "lambda-registrator-${local.suffix}"
}

resource "aws_cloudwatch_log_group" "log_group" {
  name = local.name
}

resource "aws_ecs_cluster" "cluster" {
  name               = local.name
  capacity_providers = ["FARGATE"]
}

module "preexisting-lambda" {
  source = "../tests/lambda"
  name   = "preexisting_${local.suffix}"
  tags = {
    "serverless.consul.hashicorp.com/v1alpha1/lambda/enabled" : "true",
    "serverless.consul.hashicorp.com/v1alpha1/lambda/payload-passhthrough" : "true",
  }
  region = var.region
}
