# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "name" {
  description = "Name to be used on all the resources as an identifier."
  type        = string
}

variable "ecr_image_uri" {
  description = "The private ECR image URI for consul-lambda-registrator."
  type        = string
}

variable "region" {
  description = "AWS region."
  type        = string
}

variable "ingress_cidrs" {
  description = "List of CIDRS that are permitted to access the public ingress points of the application."
  type        = list(string)
}

variable "num_ecs_apps" {
  description = "The number of upstream services to create"
  type        = number
  default     = 2
}

variable "sync_frequency_in_minutes" {
  description = "The interval EventBridge is configured to trigger consul-lambda-registrator to perform full synchronization of Lambda state with Consul."
  type        = number
  default     = 10
}

variable "consul_lambda_extension_arn" {
  description = "The ARN of the Consul Lambda extension. If empty the function will not use the extension."
  type        = string
  default     = ""
}

variable "consul_lambda_registrator_image" {
  description = "The Lambda registrator image to use. Must be provided as <registry/repository:tag>"
  type        = string
  default     = "public.ecr.aws/hashicorp/consul-lambda-registrator:0.1.0-beta4"

  validation {
    condition     = can(regex("^[a-zA-Z0-9_.-]+/[a-z0-9_.-]+/[a-z0-9_.-]+:[a-zA-Z0-9_.-]+$", var.consul_lambda_registrator_image))
    error_message = "Image format of 'consul_lambda_registrator_image' is invalid. It should be in the format 'registry/repository:tag'."
  }
}
