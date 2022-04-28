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

variable "lb_ingress_ip" {
  type = string
}
