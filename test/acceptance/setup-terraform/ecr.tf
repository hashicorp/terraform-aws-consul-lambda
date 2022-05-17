locals {
  ecr_repository_name = "lr_${local.suffix}"
  ecr_image_tag       = "latest"
  account_id          = data.aws_caller_identity.current.account_id
}

resource "aws_ecr_repository" "lambda-registrator" {
  name = local.ecr_repository_name
}

resource "null_resource" "push-lambda-registrator-to-ecr" {
  triggers = {
    always_run = timestamp()
  }

  provisioner "local-exec" {
    command = <<EOF
    aws ecr get-login-password --region ${var.region} | docker login --username AWS --password-stdin ${local.account_id}.dkr.ecr.${var.region}.amazonaws.com
    cd ../../../consul-lambda-registrator
    docker build -t ${aws_ecr_repository.lambda-registrator.repository_url}:${local.ecr_image_tag} .
    docker push ${aws_ecr_repository.lambda-registrator.repository_url}:${local.ecr_image_tag}
    EOF
  }

  depends_on = [
    aws_ecr_repository.lambda-registrator
  ]
}

data "aws_ecr_image" "lambda-registrator" {
  depends_on = [
    null_resource.push-lambda-registrator-to-ecr
  ]
  repository_name = local.ecr_repository_name
  image_tag       = local.ecr_image_tag
}
