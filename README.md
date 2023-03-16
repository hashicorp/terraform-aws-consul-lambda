<h1>
  <img src="https://raw.githubusercontent.com/hashicorp/terraform-aws-consul-lambda/main/_docs/logo.svg" align="left" height="46px" alt="Consul logo"/>
  <span>Consul on AWS Lambda</span>
</h1>

This repository holds the software for integrating AWS Lambda functions with Consul service mesh.
It contains:
- The Go code for Lambda registrator. Lambda registrator is an AWS Lambda function that automates and manages Consul service registration and de-registration for your Lambda functions.
- The Go code for the Consul Lambda extension. This is an external Lambda extension that allows your Lambda functions to call services in the Consul service mesh.
- A Terraform module for automating the deployment of Lambda registrator using Terraform.

Please refer to [our documentation](https://www.consul.io/docs/lambda) for full details on integrating AWS Lambda functions with Consul service mesh.

## Contributing

We want to create a strong community around Consul on Lambda. We will take all PRs very seriously and review for inclusion. Please read about [contributing](./CONTRIBUTING.md).

## License

This code is released under the Mozilla Public License 2.0. Please see [LICENSE](https://github.com/hashicorp/terraform-aws-consul-lambda/blob/main/LICENSE) for more details.
