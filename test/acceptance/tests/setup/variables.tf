# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "suffix" {
  type = string
}

variable "setup_suffix" {
  type = string
}

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "secure" {
  type = bool
}

variable "private_subnets" {
  type = list(string)
}

variable "public_subnets" {
  type = list(string)
}

variable "ecs_cluster_arn" {
  type = string
}

variable "log_group_name" {
  type = string
}

variable "vpc_id" {
  type = string
}

variable "security_group_id" {
  type = string
}

variable "consul_image" {
  type = string
}

variable "ecr_image_uri" {
  type    = string
  default = ""
}

variable "consul_license" {
  type    = string
  default = ""
}

variable "consul_namespace" {
  type    = string
  default = ""
}

variable "consul_partition" {
  type    = string
  default = ""
}

variable "consul_lambda_extension_arn" {
  type = string
}

variable "enable_auto_publish_ecr_image" {
  type    = bool
  default = false
}

variable "private_ecr_repo_name" {
  type    = string
  default = ""
}

variable "arch" {
  type        = string
  default     = "x86_64"
  description = "Lambda Architecture"
}

variable "consul_lambda_registrator_image" {
  description = "The Lambda registrator image to use. Must be provided as <registry/repository:tag>"
  type        = string
  default     = "public.ecr.aws/hashicorp/consul-lambda-registrator:0.1.0-beta4"

  validation {
    condition     = can(regex("^[a-zA-Z0-9_.-]+/[a-z0-9_.-]+/[a-z0-9_.-]+:[a-zA-Z0-9_.-]+$", var.consul_lambda_registrator_image))
    error_message = "Image format of 'consul_lambda_registrator_image' is invalid. It must be in the format 'registry/repository:tag'."
  }
}