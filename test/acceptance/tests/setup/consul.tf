// Generate a gossip encryption key if a secure installation.
resource "random_id" "gossip_encryption_key" {
  count       = var.secure ? 1 : 0
  byte_length = 32
}

resource "aws_secretsmanager_secret" "gossip_key" {
  count                   = var.secure ? 1 : 0
  name                    = "consul_server_${var.suffix}-gossip-encryption-key"
  recovery_window_in_days = "0"
}

resource "aws_secretsmanager_secret_version" "gossip_key" {
  count         = var.secure ? 1 : 0
  secret_id     = aws_secretsmanager_secret.gossip_key[0].id
  secret_string = random_id.gossip_encryption_key[0].b64_std
}

module "dev_consul_server" {
  consul_image                = var.consul_image
  name                        = "${local.short_name}-consul-server"
  source                      = "hashicorp/consul-ecs/aws//modules/dev-server"
  version                     = "0.5.0"
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
  launch_type           = "FARGATE"
  tls                   = var.secure
  acls                  = var.secure
  gossip_encryption_enabled = var.secure
  generate_gossip_encryption_key = false
  gossip_key_secret_arn = var.secure ? aws_secretsmanager_secret.gossip_key[0].arn : ""
  consul_license        = var.consul_license

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
