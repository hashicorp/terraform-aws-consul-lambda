locals {
  upstream_uris = join(",", [for u in var.upstreams : "http://localhost:${u.localBindPort}"])

  load_balancer = var.lb_target_group_arn != "" ? [{
    target_group_arn = var.lb_target_group_arn
    container_name   = "app"
    container_port   = 9090
  }] : []

  log_config = {
    logDriver = "awslogs"
    options = {
      awslogs-group         = var.log_group
      awslogs-region        = var.region
      awslogs-stream-prefix = var.name
    }
  }
}

resource "aws_ecs_service" "this" {
  name            = var.name
  cluster         = var.ecs_cluster
  task_definition = module.this.task_definition_arn
  desired_count   = 1
  network_configuration {
    subnets = var.subnets
  }
  launch_type            = "FARGATE"
  propagate_tags         = "TASK_DEFINITION"
  enable_execute_command = true

  dynamic "load_balancer" {
    for_each = local.load_balancer
    content {
      target_group_arn = load_balancer.value["target_group_arn"]
      container_name   = load_balancer.value["container_name"]
      container_port   = load_balancer.value["container_port"]
    }
  }
}

module "this" {
  source                    = "hashicorp/consul-ecs/aws//modules/mesh-task"
  version                   = "0.5.1"
  family                    = var.name
  port                      = "9090"
  tls                       = true
  consul_server_ca_cert_arn = var.ca_cert_arn
  gossip_key_secret_arn     = var.gossip_key_arn
  log_configuration         = local.log_config
  container_definitions = [{
    name             = "app"
    image            = "docker.mirror.hashicorp.services/nicholasjackson/fake-service:v0.21.0"
    essential        = true
    logConfiguration = local.log_config
    environment = setunion([
      {
        name  = "NAME"
        value = var.name
      },
      {
        # We need to configure the fake-service so that it does not append the original request headers
        # to the upstream request. If the original request headers are appended, this invalidates
        # the AWS signature that Envoy calculates in the request that it makes to Lambda.
        name  = "HTTP_CLIENT_APPEND_REQUEST"
        value = "false"
      }
      ],
      local.upstream_uris != "" ? [{
        name  = "UPSTREAM_URIS"
        value = local.upstream_uris
      }] : []
    )
    portMappings = [
      {
        containerPort = 9090
        hostPort      = 9090
        protocol      = "tcp"
      }
    ]
    cpu         = 0
    mountPoints = []
    volumesFrom = []
    # An ECS health check. This will be automatically synced into Consul.
    healthCheck = {
      command  = ["CMD-SHELL", "curl localhost:9090/health"]
      interval = 30
      retries  = 3
      timeout  = 5
    }
  }]
  retry_join = var.retry_join

  upstreams = var.upstreams

  additional_task_role_policies = var.additional_task_role_policies

  consul_image               = "public.ecr.aws/hashicorp/consul:1.12.4"
  consul_agent_configuration = <<EOF
connect {
  enabled = true,
  enable_serverless_plugin = true
}
EOF
}
