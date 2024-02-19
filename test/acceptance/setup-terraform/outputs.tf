# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

output "vpc_id" {
  value = module.vpc.vpc_id
}

output "security_group_id" {
  value = module.vpc.default_security_group_id
}

output "ecs_cluster_arn" {
  value = aws_ecs_cluster.cluster.arn
}

output "region" {
  value = var.region
}

output "private_subnets" {
  value = module.vpc.private_subnets
}

output "public_subnets" {
  value = module.vpc.public_subnets
}

output "log_group_name" {
  value = aws_cloudwatch_log_group.log_group.name
}

output "ecr_image_uri" {
  value = "${aws_ecr_repository.lambda-registrator.repository_url}:${local.ecr_image_tag}"
}

output "suffix" {
  value = random_string.suffix.result
}

output "consul_lambda_extension_arn" {
  value = aws_lambda_layer_version.consul_lambda_extension.arn
}

output "arch" {
  value = var.arch
}