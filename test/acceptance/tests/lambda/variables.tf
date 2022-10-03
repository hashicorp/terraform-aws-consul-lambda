variable "tags" {
  type = map(string)
}

variable "name" {
  type = string
}

variable "region" {
  type = string
}

variable "env" {
  type    = map(string)
  default = {}
}

variable "layers" {
  type    = list(string)
  default = []
}
