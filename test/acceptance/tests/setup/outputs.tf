output "consul_http_addr" {
  value = local.consul_http_addr
}

output "mesh_gateway_uri" {
  value = "${module.mesh_gateway.wan_address}:${module.mesh_gateway.wan_port}"
}
