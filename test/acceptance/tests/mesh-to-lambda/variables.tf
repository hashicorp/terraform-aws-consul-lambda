# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "tags" {
  type = map(string)
}

variable "name" {
  type = string
}

variable "region" {
  type = string
}

variable "arch" {
  type        = string
  default     = "x86_64"
  description = "Lambda Architecture"
}