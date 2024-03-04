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

variable "ecr_image_uri" {
  description = <<-EOT
  The ECR image URI for consul-lambda-registrator. The image must be in the
  same AWS region and in a private ECR repository. Due to these constraints,
  the public ECR images (https://gallery.ecr.aws/hashicorp/consul-lambda-registrator)
  cannot be used directly. We recommend either creating and using a new ECR
  repository or configuring pull through cache rules (https://docs.aws.amazon.com/AmazonECR/latest/userguide/pull-through-cache.html).
  EOT
  type        = string
  default     = ""
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

variable "private_ecr_repo_name" {
  description = "The name of the repository to republish the ECR image if one exists. If no name is passed, it is assumed that no repository exists and one needs to be created."
  type        = string
  default     = ""
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

variable "docker_host" {
  description = "The docker socket for your system"
  type        = string
  default     = "unix:///var/run/docker.sock"
}

variable "enable_auto_publish_ecr_image" {
  description = <<-EOT
    Enables automatic publishing of a public Lambda Registrator image to a private ECR repository via Docker.
    When enable_auto_publish_ecr_image is set to true, the image defined by consul_lambda_registrator_image will be pulled and published to a private ECR repository. If private_ecr_repo_name is set, that name will be used to create the private ECR repository, otherwise the default name, consul-lambda-registrator-<random-suffix>, will be used
    
    You must set at least one of ecr_image_uri or enable_auto_publish_ecr_image. If enable_auto_publish_ecr_image is set to true then ecr_image_uri is ignored.
    
    Using this method to automatically pull the public image and push it to a private ECR repository requires access to the docker command in the local environment.
    EOT
  type        = bool
  default     = false
}