resource "aws_ecs_cluster" "this" {
  name = var.name
}

resource "aws_ecs_cluster_capacity_providers" "this" {
  cluster_name       = aws_ecs_cluster.this.name
  capacity_providers = ["FARGATE"]

  default_capacity_provider_strategy {
    capacity_provider = "FARGATE"
  }
}

resource "aws_cloudwatch_log_group" "this" {
  name = var.name
}

// ECS application that calls a Lambda function in the Consul service mesh.
module "ecs_app_1" {
  source         = "./ecs-app"
  name           = "${var.name}-ecs-app-1"
  region         = var.region
  ecs_cluster    = aws_ecs_cluster.this.arn
  subnets        = module.vpc.private_subnets
  retry_join     = [module.dev_consul_server.server_dns]
  ca_cert_arn    = module.dev_consul_server.ca_cert_arn
  gossip_key_arn = module.dev_consul_server.gossip_key_arn
  log_group      = aws_cloudwatch_log_group.this.name

  upstreams = [
    {
      destinationName = "${var.name}-lambda-app-1"
      localBindPort   = 1234
    }
  ]

  additional_task_role_policies = [
    aws_iam_policy.execute_command.arn,
    aws_iam_policy.invoke_lambda.arn
  ]

  lb_target_group_arn = aws_lb_target_group.ecs_app_1_alb.arn
}

// ECS application that is called by a Lambda function in the Consul service mesh.
module "ecs_app_2" {
  source         = "./ecs-app"
  name           = "${var.name}-ecs-app-2"
  region         = var.region
  ecs_cluster    = aws_ecs_cluster.this.arn
  subnets        = module.vpc.private_subnets
  retry_join     = [module.dev_consul_server.server_dns]
  ca_cert_arn    = module.dev_consul_server.ca_cert_arn
  gossip_key_arn = module.dev_consul_server.gossip_key_arn
  log_group      = aws_cloudwatch_log_group.this.name

  additional_task_role_policies = [
    aws_iam_policy.execute_command.arn,
  ]
}

data "aws_security_group" "vpc_default" {
  name   = "default"
  vpc_id = module.vpc.vpc_id
}

resource "aws_lb" "ecs_app_1" {
  name               = "${var.name}-ecs-app-1"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.ecs_app_1_alb.id]
  subnets            = module.vpc.public_subnets
}

resource "aws_security_group" "ecs_app_1_alb" {
  name   = "${var.name}-ecs-app-1-alb"
  vpc_id = module.vpc.vpc_id

  ingress {
    description = "Access to example client application."
    from_port   = 9090
    to_port     = 9090
    protocol    = "tcp"
    cidr_blocks = var.ingress_cidrs
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group_rule" "ingress_from_client_alb_to_ecs" {
  type                     = "ingress"
  from_port                = 0
  to_port                  = 65535
  protocol                 = "tcp"
  source_security_group_id = aws_security_group.ecs_app_1_alb.id
  security_group_id        = data.aws_security_group.vpc_default.id
}

resource "aws_security_group_rule" "ingress_from_server_alb_to_ecs" {
  type                     = "ingress"
  from_port                = 8500
  to_port                  = 8500
  protocol                 = "tcp"
  source_security_group_id = module.dev_consul_server.lb_security_group_id
  security_group_id        = data.aws_security_group.vpc_default.id
}

resource "aws_lb_target_group" "ecs_app_1_alb" {
  name                 = "${var.name}-ecs-app-1"
  port                 = 9090
  protocol             = "HTTP"
  vpc_id               = module.vpc.vpc_id
  target_type          = "ip"
  deregistration_delay = 10
  health_check {
    path                = "/health"
    healthy_threshold   = 2
    unhealthy_threshold = 10
    timeout             = 30
    interval            = 60
  }
}

resource "aws_lb_listener" "ecs_app_1" {
  load_balancer_arn = aws_lb.ecs_app_1.arn
  port              = "9090"
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.ecs_app_1_alb.arn
  }
}

// Policy that allows ECS tasks to invoke the Lambda function.
resource "aws_iam_policy" "invoke_lambda" {
  name   = "${var.name}-ecs-invoke-lambda"
  path   = "/"
  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "lambda:InvokeFunction"
      ],
      "Resource": "${module.lambda_app_1.arn}"
    }
  ]
}
EOF

}

// Policy that allows execution of remote commands in ECS tasks.
resource "aws_iam_policy" "execute_command" {
  name   = "${var.name}-ecs-execute-command"
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
