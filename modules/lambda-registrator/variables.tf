# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "name" {
  description = "This is used to name Lambda registrator’s Lambda function and to construct the Identity and Access Management (IAM) role and policy names used by the Lambda function."
  type        = string
}

variable "consul_http_addr" {
  description = "The HTTP(S) address of the Consul server. This must be a full URL, including port and scheme, e.g. https://consul.example.com:8501."
  type        = string
}

variable "consul_datacenter" {
  description = "The Consul datacenter that the Lambda registrator is part of."
  type        = string
  default     = ""
}

variable "consul_ca_cert_path" {
  description = "The Parameter Store path containing the Consul server CA certificate."
  type        = string
  default     = ""
}

variable "consul_http_token_path" {
  description = "The Parameter Store path containing the Consul ACL token."
  type        = string
  default     = ""
}

variable "consul_extension_data_prefix" {
  description = "The path within Parameter Store where Lambda registrator will write the Consul Lambda extension data. If this is unset, Lambda registrator will not write Consul data to Parameter Store."
  type        = string
  default     = ""
}

variable "consul_extension_data_tier" {
  description = <<-EOT
  The tier to use for storing data in Parameter Store.
  Refer to the Parameter Store documentation for applicable values (https://docs.aws.amazon.com/systems-manager/latest/userguide/parameter-store-advanced-parameters.html).
  If this is unset the default tier will be used.
  EOT
  type        = string
  default     = ""
}

variable "node_name" {
  description = "The Consul node that Lambdas will be registered to."
  type        = string
  default     = "lambdas"
}

variable "enterprise" {
  description = "Determines if Consul Enterprise is being used [Consul Enterprise]."
  type        = bool
  default     = false
}

variable "partitions" {
  description = "Specifies the partitions that Lambda registrator will manage [Consul Enterprise]."
  type        = list(string)
  default     = []
}

variable "timeout" {
  description = "The maximum number of seconds Lambda registrator can run before timing out."
  type        = number
  default     = 30
}

variable "reserved_concurrent_executions" {
  description = "The amount of reserved concurrent executions for Lambda registrator."
  type        = number
  default     = -1
}


variable "sync_frequency_in_minutes" {
  description = "The interval EventBridge is configured to trigger full synchronizations."
  type        = number
  default     = 10
}

variable "subnet_ids" {
  description = "List of subnet IDs associated with Lambda registrator"
  type        = list(string)
  default     = []
}

variable "security_group_ids" {
  description = "List of security group IDs associated with Lambda registrator"
  type        = list(string)
  default     = []
}

variable "tags" {
  description = "Additional tags to set on the Lambda registrator."
  type        = map(string)
  default     = {}
}
variable "region" {
  type        = string
  description = "AWS region to deploy Lambda registrator."
}

variable "private_repo_name" {
  description = "The name of the repository to republish the ECR image if one exists. If no name is passed, it is assumed that no repository exists and one needs to be created. Note :- If 'pull_through' is true this variable is ignored."
  type        = string
  default     = "consul-lambda-registrator"
}

variable "enable_pull_through_cache" {
  description = "Flag to determine if a pull-through cache method will be used to obtain the appropriate ECR image"
  type        = bool
  default     = false
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

variable "docker_host" {
  description = "The docker socket for your system"
  type        = string
  default     =  "unix:///var/run/docker.sock"
}

variable ecr_repository_prefix {
  description = "The repository namespace to use when caching images from the source registry"
  type        = string
  default     =  "ecr-public"
}

variable upstream_registry_url {
  description = "The public registry url"
  type        = string
  default     =  "public.ecr.aws"
}