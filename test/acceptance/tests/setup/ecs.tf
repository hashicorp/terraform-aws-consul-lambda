# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "aws_ecs_service" "test_client" {
  name            = "test_client_${var.suffix}"
  cluster         = var.ecs_cluster_arn
  task_definition = module.test_client.task_definition_arn
  desired_count   = 1
  network_configuration {
    subnets = var.private_subnets
  }
  launch_type            = "FARGATE"
  propagate_tags         = "TASK_DEFINITION"
  enable_execute_command = true
}

module "test_client" {
  source  = "hashicorp/consul-ecs/aws//modules/mesh-task"
  version = "0.6.0"
  family  = "test_client_${var.suffix}"
  port    = "9090"
  container_definitions = [{
    name      = "basic"
    image     = "docker.mirror.hashicorp.services/nicholasjackson/fake-service:v0.21.0"
    essential = true
    environment = [
      {
        name  = "UPSTREAM_URIS"
        value = var.consul_partition == "" ? "http://localhost:1234,http://localhost:1235,http://localhost:1236,http://localhost:2345" : "http://localhost:1234,http://localhost:1235,http://localhost:1236"
      },
      {
        # We need to configure the fake-service so that it does not append the original request headers
        # to the upstream request. If the original request headers are appended, this invalidates
        # the AWS signature that Envoy calculates in the request that it makes to Lambda.
        name  = "HTTP_CLIENT_APPEND_REQUEST"
        value = "false"
      }
    ]
    portMappings = [
      {
        containerPort = 9090
        hostPort      = 9090
        protocol      = "tcp"
      }
    ]
    linuxParameters = {
      initProcessEnabled = true
    }
  }]
  retry_join = [module.dev_consul_server.server_dns]
  upstreams = [
    {
      destinationName      = "mesh_to_lambda_example_${var.suffix}"
      localBindPort        = 1234
      destinationPartition = var.consul_partition
      destinationNamespace = var.consul_namespace
    },
    {
      destinationName      = "mesh_to_lambda_example_${var.suffix}-dev"
      localBindPort        = 1235
      destinationPartition = var.consul_partition
      destinationNamespace = var.consul_namespace
    },
    {
      destinationName      = "mesh_to_lambda_example_${var.suffix}-prod"
      localBindPort        = 1236
      destinationPartition = var.consul_partition
      destinationNamespace = var.consul_namespace
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

  consul_image     = var.consul_image
  consul_http_addr = local.consul_http_addr
  consul_namespace = var.consul_namespace
  consul_partition = var.consul_partition

  additional_task_role_policies = [
    aws_iam_policy.execute_command.arn,
    aws_iam_policy.invoke_lambda.arn
  ]
  consul_agent_configuration = <<-EOT
  log_level = "debug"
  EOT

  tls                       = true
  consul_server_ca_cert_arn = module.dev_consul_server.ca_cert_arn
  consul_https_ca_cert_arn  = module.dev_consul_server.ca_cert_arn
  acls                      = var.secure
  gossip_key_secret_arn     = var.secure ? module.dev_consul_server.gossip_key_arn : ""
}

// Policy to allow `aws execute-command`
resource "aws_iam_policy" "execute_command" {
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

resource "aws_iam_policy" "invoke_lambda" {
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
  count   = var.secure ? 1 : 0
  source  = "hashicorp/consul-ecs/aws//modules/acl-controller"
  version = "0.6.0"
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
  consul_server_http_addr           = local.consul_http_addr
  consul_server_ca_cert_arn         = module.dev_consul_server.ca_cert_arn
  ecs_cluster_arn                   = var.ecs_cluster_arn
  region                            = var.region
  subnets                           = var.private_subnets
  name_prefix                       = var.suffix
  consul_partitions_enabled         = local.enterprise
  consul_partition                  = var.consul_partition
}
