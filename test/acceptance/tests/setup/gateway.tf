# Copyright IBM Corp. 2022, 2025
# SPDX-License-Identifier: MPL-2.0

module "mesh_gateway" {
  source  = "hashicorp/consul-ecs/aws//modules/gateway-task"
  version = "0.9.3"

  kind                          = "mesh-gateway"
  family                        = "mesh-gateway-${var.suffix}"
  ecs_cluster_arn               = var.ecs_cluster_arn
  subnets                       = var.private_subnets
  security_groups               = [var.security_group_id]
  consul_server_hosts           = module.dev_consul_server.server_dns
  consul_partition              = var.consul_partition
  tls                           = true
  consul_ca_cert_arn            = module.dev_consul_server.ca_cert_arn
  consul_https_ca_cert_arn      = module.dev_consul_server.ca_cert_arn
  acls                          = var.secure
  lb_enabled                    = true
  lb_subnets                    = var.public_subnets
  lb_vpc_id                     = var.vpc_id
  additional_task_role_policies = [aws_iam_policy.execute_command.arn]
  # Transparent proxy is not supported for FARGATE launch type
  enable_transparent_proxy = false

  log_configuration = {
    logDriver = "awslogs"
    options = {
      awslogs-group         = var.log_group_name
      awslogs-region        = var.region
      awslogs-stream-prefix = "mesh-gateway"
    }
  }
}
