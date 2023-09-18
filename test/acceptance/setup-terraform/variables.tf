# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "region" {
  default     = "us-west-2"
  description = "AWS region"
}

variable "enable_auto_publish_ecr_image" {
  description = "enables auto pushing public image to private ecr repo if set to true"
  type        = bool
  default     = false
}