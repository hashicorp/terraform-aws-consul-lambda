# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

locals {
  require_ecr_image_uri_or_enable_auto_publish_ecr_image_set = var.ecr_image_uri == "" && var.enable_auto_publish_ecr_image == false ? file("ERROR: either ecr_image_uri or enable_auto_publish_ecr_image must be set") : null
}