# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

schema = 1
artifacts {
  container = [
    "terraform-aws-consul-lambda_release-default_linux_amd64_${version}_${commit_sha}.docker.dev.tar",
    "terraform-aws-consul-lambda_release-default_linux_amd64_${version}_${commit_sha}.docker.tar",
    "terraform-aws-consul-lambda_release-default_linux_arm64_${version}_${commit_sha}.docker.dev.tar",
    "terraform-aws-consul-lambda_release-default_linux_arm64_${version}_${commit_sha}.docker.tar",
  ]
}
