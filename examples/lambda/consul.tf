module "dev_consul_server" {
  name                        = "${var.name}-consul-server"
  source                      = "hashicorp/consul-ecs/aws//modules/dev-server"
  version                     = "0.5.1"
  ecs_cluster_arn             = aws_ecs_cluster.this.arn
  subnet_ids                  = module.vpc.private_subnets
  vpc_id                      = module.vpc.vpc_id
  tls                         = true
  gossip_encryption_enabled   = true
  lb_enabled                  = true
  lb_subnets                  = module.vpc.public_subnets
  lb_ingress_rule_cidr_blocks = var.ingress_cidrs
  log_configuration = {
    logDriver = "awslogs"
    options = {
      awslogs-group         = aws_cloudwatch_log_group.this.name
      awslogs-region        = var.region
      awslogs-stream-prefix = "consul-server"
    }
  }
  launch_type = "FARGATE"

  consul_image = "public.ecr.aws/hashicorp/consul:1.12.4"
}

resource "aws_security_group_rule" "consul_server_ingress" {
  description              = "Access to Consul dev server from default security group"
  type                     = "ingress"
  from_port                = 0
  to_port                  = 0
  protocol                 = "-1"
  source_security_group_id = module.vpc.default_security_group_id
  security_group_id        = module.dev_consul_server.security_group_id
}

