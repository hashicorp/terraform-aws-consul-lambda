# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "name" {
  description = "The name of this Lambda function. This is also the service name that will get registered with Consul."
  type        = string
}

variable "file_path" {
  description = "The path on the local filesystem to the Lambda function zip file."
  type        = string
}

variable "role_arn" {
  description = "The ARN of the role to use for the Lambda function."
  type        = string
}

variable "tags" {
  description = "Additional tags to apply to the Lambda function."
  type        = map(string)
  default     = {}
}

variable "runtime" {
  description = "Lambda runtime."
  type        = string
  default     = "go1.x"
}

variable "handler" {
  description = "Lambda function handler."
  type        = string
  default     = "main"
}

variable "consul_datacenter" {
  description = "The Consul datacenter that the Lambda function belongs to."
  type        = string
  default     = ""
}

variable "consul_partition" {
  description = "The Consul admin partition that the Lambda function belongs to [Enterprise]."
  type        = string
  default     = ""
}

variable "consul_namespace" {
  description = "The Consul namespace that the Lambda function belongs to [Enterprise]."
  type        = string
  default     = ""
}

variable "consul_upstreams" {
  description = "List of Consul service mesh upstreams the Lambda function will call."
  type        = list(string)
  default     = []
}

variable "consul_lambda_extension_arn" {
  description = "The ARN of the Consul-Lambda extension layer. The Consul-Lambda extension layer is required for Lambda functions that call into the Consul service mesh. It is not required for Lambda functions that are only called by mesh services."
  type        = string
  default     = ""
}

variable "consul_extension_data_prefix" {
  description = "The path prefix to the location within the parameter store where the Consul-Lambda extension data is stored."
  type        = string
  default     = ""
}

variable "consul_refresh_frequency" {
  description = "The amount of time to wait between refreshes of Consul state. Provided in Go time.Duration format (e.g.: 5m)."
  type        = string
  default     = ""
}

variable "consul_mesh_gateway_uri" {
  description = "The URI of the mesh gateway. This is required for Lambda functions that call into the Consul service mesh."
  type        = string
  default     = ""
}

variable "payload_passthrough" {
  description = "Whether to transform the request (headers and body) to a JSON payload or pass it as is. When set to false the request is transformed. When set to true the payload is passed through as is."
  type        = string
  default     = "true"
}

variable "invocation_mode" {
  description = "The invocation mode. "
  type        = string
  default     = "SYNCHRONOUS"
  validation {
    condition     = contains(["SYNCHRONOUS", "ASYNCHRONOUS"], var.invocation_mode)
    error_message = "Invocation_mode must be one of SYNCHRONOUS or ASYNCHRONOUS."
  }
}

variable "aliases" {
  description = "List of aliases to register the Lambda function as within Consul."
  type        = list(string)
  default     = []
}

variable "env" {
  description = "Additional environment variables to configure for the Lambda function."
  type        = map(string)
  default     = {}
}

variable "layers" {
  description = "Additional layers to add to the Lambda function."
  type        = list(string)
  default     = []
}

