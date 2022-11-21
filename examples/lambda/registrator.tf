# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

module "consul_lambda_registrator" {
  source                       = "../../modules/lambda-registrator"
  name                         = "${var.name}-lambda-registrator"
  consul_http_addr             = "http://${module.dev_consul_server.server_dns}:8500"
  consul_extension_data_prefix = "/${var.name}"
  subnet_ids                   = module.vpc.private_subnets
  security_group_ids           = [module.vpc.default_security_group_id]
  sync_frequency_in_minutes    = 1
  pull_through                 = false
}
