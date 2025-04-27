# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

schema = 1
artifacts {
  zip = [
    "consul-lambda-extension_${version}_linux_amd64.zip",
    "consul-lambda-extension_${version}_linux_arm64.zip",
    "consul-lambda-registrator_${version}_linux_amd64.zip",
    "consul-lambda-registrator_${version}_linux_arm64.zip",
  ]
  container = [
    "terraform-aws-consul-lambda_release-default_linux_amd64_${version}_${commit_sha}.docker.dev.tar",
    "terraform-aws-consul-lambda_release-default_linux_amd64_${version}_${commit_sha}.docker.tar",
    "terraform-aws-consul-lambda_release-default_linux_arm64_${version}_${commit_sha}.docker.dev.tar",
    "terraform-aws-consul-lambda_release-default_linux_arm64_${version}_${commit_sha}.docker.tar",
  ]
}
