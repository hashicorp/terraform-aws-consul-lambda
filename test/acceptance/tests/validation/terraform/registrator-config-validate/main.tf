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

module "lambda-registration" {
  source                        = "../../../../../../modules/lambda-registrator"
  name                          = var.name
  ecr_image_uri                 = var.ecr_image_uri
  consul_http_addr              = var.consul_http_addr
  enable_auto_publish_ecr_image = var.enable_auto_publish_ecr_image
}

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "ecr_image_uri" {
  type    = string
  default = ""
}
variable "name" {
  type    = string
  default = ""
}
variable "enable_auto_publish_ecr_image" {
  description = "enables auto pushing public image to private ecr repo if set to true"
  type        = bool
  default     = false
}

variable "consul_http_addr" {
  type = string
}