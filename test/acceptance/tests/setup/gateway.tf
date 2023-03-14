# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

module "mesh_gateway" {
  //TODO source                        = "hashicorp/consul-ecs/aws//modules/gateway-task"
  //TODO version                       = "0.5.2"
  consul_ecs_image = "docker.mirror.hashicorp.services/hashicorpdev/consul-ecs:latest"
  source           = "github.com/hashicorp/terraform-aws-consul-ecs//modules/gateway-task?ref=839c2aebd4dc38e579297b823acadd71312d5e7c"

  kind                          = "mesh-gateway"
  family                        = "mesh-gateway-${var.suffix}"
  ecs_cluster_arn               = var.ecs_cluster_arn
  subnets                       = var.private_subnets
  security_groups               = [var.security_group_id]
  retry_join                    = [module.dev_consul_server.server_dns]
  consul_image                  = var.consul_image
  consul_http_addr              = local.consul_http_addr
  consul_partition              = var.consul_partition
  tls                           = true
  consul_server_ca_cert_arn     = module.dev_consul_server.ca_cert_arn
  consul_https_ca_cert_arn      = module.dev_consul_server.ca_cert_arn
  acls                          = var.secure
  gossip_key_secret_arn         = var.secure ? module.dev_consul_server.gossip_key_arn : ""
  lb_enabled                    = true
  lb_subnets                    = var.public_subnets
  lb_vpc_id                     = var.vpc_id
  additional_task_role_policies = [aws_iam_policy.execute_command.arn]

  log_configuration = {
    logDriver = "awslogs"
    options = {
      awslogs-group         = var.log_group_name
      awslogs-region        = var.region
      awslogs-stream-prefix = "mesh-gateway"
    }
  }

  consul_agent_configuration = <<-EOT
  EOT
}
