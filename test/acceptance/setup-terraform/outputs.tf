output "vpc_id" {
  value = module.vpc.vpc_id
}

output "ecs_cluster_arn" {
  value = aws_ecs_cluster.cluster.arn
}

output "region" {
  value = var.region
}

output "subnets" {
  value = module.vpc.private_subnets
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
