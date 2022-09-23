locals {
  tags = merge(
    { "serverless.consul.hashicorp.com/v1alpha1/lambda/enabled" : "true" },
    var.consul_datacenter != "" ?
    { "serverless.consul.hashicorp.com/v1alpha1/lambda/datacenter" : var.consul_datacenter } : {},
    var.consul_partition != "" ?
    { "serverless.consul.hashicorp.com/v1alpha1/lambda/partition" : var.consul_partition } : {},
    var.consul_namespace != "" ?
    { "serverless.consul.hashicorp.com/v1alpha1/lambda/namespace" : var.consul_namespace } : {},
    var.payload_passthrough != "" ?
    { "serverless.consul.hashicorp.com/v1alpha1/lambda/payload-passthrough" : var.payload_passthrough } : {},
    length(var.aliases) > 0 ?
    { "serverless.consul.hashicorp.com/v1alpha1/lambda/aliases" : join("+", var.aliases) } : {}
  )

  layers = concat(
    var.layers,
    var.consul_lambda_extension_arn != "" ? [var.consul_lambda_extension_arn] : [],
  )
}

resource "aws_lambda_function" "lambda_app" {
  function_name    = var.name
  role             = var.role_arn
  filename         = var.file_path
  source_code_hash = filebase64sha256(var.file_path)
  handler          = var.handler
  runtime          = var.runtime
  tags             = merge(local.tags, var.tags)
  layers           = local.layers

  environment {
    variables = merge(var.env,
      var.consul_extension_data_prefix != "" ? {
        CONSUL_EXTENSION_DATA_PREFIX = var.consul_extension_data_prefix
      } : {},
      var.consul_mesh_gateway_uri != "" ? {
        CONSUL_MESH_GATEWAY_URI = var.consul_mesh_gateway_uri
      } : {},
      var.consul_refresh_frequency != "" ? {
        CONSUL_REFRESH_FREQUENCY = var.consul_refresh_frequency
      } : {},
      var.consul_datacenter != "" ? {
        CONSUL_DATACENTER = var.consul_datacenter
      } : {},
      var.consul_namespace != "" ? {
        CONSUL_SERVICE_NAMESPACE = var.consul_namespace
      } : {},
      var.consul_partition != "" ? {
        CONSUL_SERVICE_PARTITION = var.consul_partition
      } : {},
      length(var.consul_upstreams) > 0 ? {
        CONSUL_SERVICE_UPSTREAMS = join(",", var.consul_upstreams)
      } : {}
    )
  }
}
