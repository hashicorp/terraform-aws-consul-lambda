# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

module "dev_consul_server" {
  consul_image                = var.consul_image
  name                        = "${local.short_name}-consul-server"
  source                      = "hashicorp/consul-ecs/aws//modules/dev-server"
  version                     = "0.9.3"
  ecs_cluster_arn             = var.ecs_cluster_arn
  subnet_ids                  = var.private_subnets
  vpc_id                      = var.vpc_id
  lb_enabled                  = false
  service_discovery_namespace = "consul-${var.suffix}"

  log_configuration = {
    logDriver = "awslogs"
    options = {
      awslogs-group         = var.log_group_name
      awslogs-region        = var.region
      awslogs-stream-prefix = "consul-server"
    }
  }
  launch_type    = "FARGATE"
  tls            = true
  acls           = var.secure
  consul_license = var.consul_license
}

data "aws_security_group" "vpc_default" {
  vpc_id = var.vpc_id

  filter {
    name   = "group-name"
    values = ["default"]
  }
}

resource "aws_security_group_rule" "consul_server_ingress" {
  description              = "Access to Consul dev server from default security group"
  type                     = "ingress"
  from_port                = 0
  to_port                  = 0
  protocol                 = "-1"
  source_security_group_id = data.aws_security_group.vpc_default.id
  security_group_id        = module.dev_consul_server.security_group_id
}
