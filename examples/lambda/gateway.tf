module "mesh_gateway" {
  source                        = "hashicorp/consul-ecs/aws//modules/gateway-task"
  version                       = "0.5.1"
  kind                          = "mesh-gateway"
  family                        = "mesh-gateway"
  ecs_cluster_arn               = aws_ecs_cluster.this.arn
  subnets                       = module.vpc.private_subnets
  security_groups               = [module.vpc.default_security_group_id]
  retry_join                    = [module.dev_consul_server.server_dns]
  tls                           = true
  consul_server_ca_cert_arn     = module.dev_consul_server.ca_cert_arn
  gossip_key_secret_arn         = module.dev_consul_server.gossip_key_arn
  lb_enabled                    = true
  lb_subnets                    = module.vpc.public_subnets
  lb_vpc_id                     = module.vpc.vpc_id
  additional_task_role_policies = [aws_iam_policy.execute_command.arn]

  // TODO: remove? I think this is only required for ACLs
  # consul_http_addr              = "http://${module.dev_consul_server.server_dns}:8500"
  # consul_https_ca_cert_arn      = module.dev_consul_server.ca_cert_arn

  log_configuration = {
    logDriver = "awslogs"
    options = {
      awslogs-group         = aws_cloudwatch_log_group.this.name
      awslogs-region        = var.region
      awslogs-stream-prefix = "mesh-gateway"
    }
  }

  consul_image               = "public.ecr.aws/hashicorp/consul:1.12.4"
  consul_agent_configuration = <<EOF
connect {
  enabled = true,
  enable_serverless_plugin = true
}
EOF
}
