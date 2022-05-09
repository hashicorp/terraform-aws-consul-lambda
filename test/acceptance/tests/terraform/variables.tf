variable "suffix" {
  type = string
}

variable "region" {
  type    = string
  default = "us-east-1"
}

variable "acls" {
  type    = bool
  default = false
}

variable "tls" {
  type    = bool
  default = false
}

variable "subnets" {
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

variable "ecr_image_uri" {
  type = string
}
