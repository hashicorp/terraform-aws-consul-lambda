variable "name" {
  description = "This is used to name Lambda registrator’s Lambda function and to construct the Identity and Access Management (IAM) role and policy names used by the Lambda function."
  type        = string
}

variable "consul_http_addr" {
  description = "The HTTP(S) address of the Consul server. This must be a full URL, including port and scheme, e.g. https://consul.example.com:8501."
  type        = string
}

variable "consul_ca_cert_path" {
  description = "The Parameter Store path containing the Consul server CA certificate."
  type        = string
}

variable "consul_http_token_path" {
  description = "The Parameter Store path containing the Consul ACL token."
  type        = string
}

variable "node_name" {
  description = "The Consul node that Lambda functions will be registered to."
  type        = string
  default     = "lambdas"
}

variable "enterprise" {
  description = "Determines if Consul Enterprise is being used [Consul Enterprise]."
  type        = bool
  default     = false
}

variable "admin_partitions" {
  description = "Specifies the admin partitions that Lambda function registrator will manage [Consul Enterprise]."
  type        = list(string)
  default     = var.enterprise ? [] : null
}

variable "timeout" {
  description = "The maximum number of seconds Lambda function registrator can run before timing out."
  type        = number
  default     = 30
}

variable "reserved_concurrent_executions" {
  description = "The amount of reserved concurrent executions for Lambda function registrator."
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
}

variable "sync_frequency_in_minutes" {
  description = "The interval EventBridge is configured to trigger full synchronizations."
  type        = number
  default     = 10
}

variable "subnet_ids" {
  description = "List of subnet IDs associated with Lambda function registrator"
  type        = list(string)
}

variable "security_group_ids" {
  description = "List of security group IDs associated with Lambda function registrator"
  type        = list(string)
}
