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
  default     = ""
}

variable "consul_http_token_path" {
  description = "The Parameter Store path containing the Consul ACL token."
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

variable "ecr_image_uri" {
  description = "lambda-registrator Docker image."
  type        = string
  // TODO add a default when we publish this somewhere.
}
