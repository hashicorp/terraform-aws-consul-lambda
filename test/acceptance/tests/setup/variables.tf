variable "suffix" {
  type = string
}

variable "setup_suffix" {
  type = string
}

variable "region" {
  type    = string
  default = "us-east-1"
}

variable "secure" {
  type = bool
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

variable "consul_image" {
  type    = string
  default = "ghcr.io/erichaberkorn/consul:lambda-demo"
}

variable "ecr_image_uri" {
  type = string
}
