output "consul_ui" {
  value = "http://${module.dev_consul_server.lb_dns_name}:8500"
}

output "ecs_app_1_ui" {
  value = "http://${aws_lb.ecs_app_1.dns_name}:9090/ui"
}
