resource "aws_ecs_service" "test_client" {
  name            = "test_client_${var.suffix}"
  cluster         = var.ecs_cluster_arn
  task_definition = module.test_client.task_definition_arn
  desired_count   = 1
  network_configuration {
    subnets = var.subnets
  }
  launch_type            = "FARGATE"
  propagate_tags         = "TASK_DEFINITION"
  enable_execute_command = true
}

module "test_client" {
  source = "hashicorp/consul-ecs/aws//modules/mesh-task"
  family = "test_client_${var.suffix}"
  container_definitions = [{
    name      = "basic"
    image     = "docker.mirror.hashicorp.services/nicholasjackson/fake-service:v0.21.0"
    essential = true
    environment = [
      {
        name  = "UPSTREAM_URIS"
        value = "http://localhost:1234"
      }
    ]
    linuxParameters = {
      initProcessEnabled = true
    }
  }]
  retry_join = [module.dev_consul_server.server_dns]
  upstreams = [
    {
      destinationName = "example_${var.suffix}"
      localBindPort   = 1234
    },
    {
      destinationName = "example_${var.suffix}-dev"
      localBindPort   = 1235
    },
    {
      destinationName = "example_${var.suffix}-prod"
      localBindPort   = 1236
    },
    {
      destinationName = "preexisting_${var.setup_suffix}"
      localBindPort   = 2345
    }
  ]
  log_configuration = {
    logDriver = "awslogs"
    options = {
      awslogs-group         = var.log_group_name
      awslogs-region        = var.region
      awslogs-stream-prefix = "test_client_${var.suffix}"
    }
  }
  outbound_only = true

  consul_image                   = var.consul_image
  consul_server_ca_cert_arn      = module.dev_consul_server.ca_cert_arn
  consul_client_token_secret_arn = var.secure ? module.acl_controller[0].client_token_secret_arn : ""

  additional_task_role_policies = [
    aws_iam_policy.execute-command.arn,
    aws_iam_policy.invoke-lambda.arn
  ]
  consul_agent_configuration = <<-EOT
  log_level = "debug"
  connect {
    enable_serverless_plugin = true
  }
  EOT
  acls                       = var.secure
  acl_secret_name_prefix     = var.suffix
  tls                        = var.secure
  gossip_key_secret_arn      = var.secure ? aws_secretsmanager_secret.gossip_key[0].arn : ""
}

// Policy to allow `aws execute-command`
resource "aws_iam_policy" "execute-command" {
  name   = "ecs-execute-command-${var.suffix}"
  path   = "/"
  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ssmmessages:CreateControlChannel",
        "ssmmessages:CreateDataChannel",
        "ssmmessages:OpenControlChannel",
        "ssmmessages:OpenDataChannel"
      ],
      "Resource": [
        "*"
      ]
    }
  ]
}
EOF
}

resource "aws_iam_policy" "invoke-lambda" {
  name   = "ecs-invoke-lambda-${var.suffix}"
  path   = "/"
  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "lambda:InvokeFunction",
      "Resource": "*"
    }
  ]
}
EOF
}

resource "aws_iam_role" "execution" {
  name = "test_server_${var.suffix}_execution_role"
  path = "/ecs/"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
      }
    ]
  })
}

module "acl_controller" {
  count  = var.secure ? 1 : 0
  source = "hashicorp/consul-ecs/aws//modules/acl-controller"
  log_configuration = {
    logDriver = "awslogs"
    options = {
      awslogs-group         = var.log_group_name
      awslogs-region        = var.region
      awslogs-stream-prefix = "consul-acl-controller"
    }
  }
  launch_type                       = "FARGATE"
  consul_bootstrap_token_secret_arn = module.dev_consul_server.bootstrap_token_secret_arn
  consul_server_http_addr           = "https://${module.dev_consul_server.server_dns}:8501"
  consul_server_ca_cert_arn         = module.dev_consul_server.ca_cert_arn
  ecs_cluster_arn                   = var.ecs_cluster_arn
  region                            = var.region
  subnets                           = var.subnets
  name_prefix                       = var.suffix
}
