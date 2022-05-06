module "dev_consul_server" {
  name            = "${local.short_name}-consul-server"
  source          = "hashicorp/consul-ecs/aws//modules/dev-server"
  ecs_cluster_arn = var.ecs_cluster_arn
  subnet_ids      = var.subnets
  vpc_id          = var.vpc_id
  lb_enabled      = false
  log_configuration = {
    logDriver = "awslogs"
    options = {
      awslogs-group         = var.log_group_name
      awslogs-region        = var.region
      awslogs-stream-prefix = "consul-server"
    }
  }
  launch_type = "FARGATE"
  tls         = var.tls
  acls        = var.acls
}
