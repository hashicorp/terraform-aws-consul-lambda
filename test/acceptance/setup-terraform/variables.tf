# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "region" {
  default     = "us-west-2"
  description = "AWS region"
}

variable "arch" {
  type        = string
  default     = "amd64"
  description = "Build Architecture"
}