resource "null_resource" "build_lambda_function" {
  triggers = {
    always_run = timestamp()
  }
  provisioner "local-exec" {
    command = <<EOT
cd ../tests/lambda
GOOS=linux go build main.go
zip example.zip main
EOT
  }
}

module "preexisting-lambda" {
  source = "../tests/lambda"
  providers = {
    aws.provider = aws.provider
  }
  name = "preexisting_${local.suffix}"
  tags = {
    "serverless.consul.hashicorp.com/v1alpha1/lambda/enabled" : "true",
    "serverless.consul.hashicorp.com/v1alpha1/lambda/payload-passthrough" : "true",
  }
  region = var.region

  depends_on = [null_resource.build_lambda_function]
}

resource "null_resource" "build_lambda_extension" {
  triggers = {
    always_run = timestamp()
  }
  provisioner "local-exec" {
    command = <<EOT
cd ../../../consul-lambda/consul-lambda-extension
mkdir extensions
GOOS=linux go build -o extensions/ .
zip -r consul-lambda-extension.zip extensions/
rm -rf extensions/
cd -
mv ../../../consul-lambda/consul-lambda-extension/consul-lambda-extension.zip .
EOT
  }
}

resource "aws_lambda_layer_version" "consul_lambda_extension" {
  layer_name  = "consul-lambda-extension-${local.suffix}"
  filename    = "consul-lambda-extension.zip"
  description = "Consul service mesh extension for AWS Lambda"
  depends_on  = [null_resource.build_lambda_extension]
}
