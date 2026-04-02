# Copyright IBM Corp. 2022, 2025
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