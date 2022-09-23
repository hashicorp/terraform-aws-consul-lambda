variable "name" {
  type = string
}

variable "region" {
  type = string
}

variable "ecs_cluster" {
  type = string
}

variable "subnets" {
  type = list(string)

}

variable "retry_join" {
  type = list(string)
}

variable "log_group" {
  type = string
}

variable "ca_cert_arn" {
  type = string
}

variable "gossip_key_arn" {
  type = string
}

variable "additional_task_role_policies" {
  type    = list(string)
  default = []
}

variable "upstreams" {
  type = set(object({
    destinationName = string
    localBindPort   = number
  }))

  default = []
}

variable "lb_target_group_arn" {
  type    = string
  default = ""
}
