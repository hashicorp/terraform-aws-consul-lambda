## 0.1.0-beta5 (Mar 04, 2024)

FEATURES
* Add support for storing parameter values greater than 4 KB. The `lambda-registrator` module and source code have been updated to accept a configurable value for the [SSM parameter tier](https://docs.aws.amazon.com/systems-manager/latest/userguide/parameter-store-advanced-parameters.html). This allows users to choose if they want to use the `Advanced` tier feature. Charges apply for the `Advanved` tier so if the tier is not expressly set to `Advanced`, then the `Standard` tier will be used. Using the `Advanced` tier allows for parameter values up to 8 KB. The Lambda-registrator Terraform module can be configured using the new `consul_extension_data_tier` variable.
  [[GH-78]](https://github.com/hashicorp/terraform-aws-consul-lambda/pull/78)

* Add support for pushing `consul-lambda-registrator` public image to private ecr repo through terraform.
  [[GH-82]](https://github.com/hashicorp/terraform-aws-consul-lambda/pull/82)

* Add  arm64 support to `consul-lambda-registrator` and `consul-lambda-extension`.
  [[GH-90]](https://github.com/hashicorp/terraform-aws-consul-lambda/pull/90)

## 0.1.0-beta4 (Apr 28, 2023)

IMPROVEMENTS
* Pin the version of the `terraform-aws-modules/eventbridge/aws` module to v1.17.3. This ensures the selection of the eventbridge module is deterministic when using the `lambda-registrator` Terraform module.
  [[GH-70]](https://github.com/hashicorp/terraform-aws-consul-lambda/pull/70)

BUG FIXES
* Disable Cgo compilation for Lambda registrator and extension. Compiling without `CGO_ENABLED=0` on Go 1.20 [causes an issue](https://github.com/hashicorp/terraform-aws-consul-lambda/issues/57) that does not allow Lambda registrator or the Lambda extension to execute within the AWS Lambda runtime.
  [[GH-68]](https://github.com/hashicorp/terraform-aws-consul-lambda/pull/68)

## 0.1.0-beta3 (Mar 16, 2023)

**Note**: The `0.1.0-beta3` release contains a breaking bug that does not allow Lambda registrator or the Lambda extension to execute within the AWS Lambda runtime. For Consul versions >= 1.15.0, use the `0.1.0-beta4` release. For Consul versions < 1.15.0, use the `0.1.0-beta2` release.

BREAKING CHANGES
* `EnvoyExtensions` configuration was released in Consul 1.15.0 and is now used to configure Lambda functions.
  [[GH-51]](https://github.com/hashicorp/terraform-aws-consul-lambda/pull/51)

FEATURES
* Update minimum go version for project to 1.20 [[GH-1908]](https://github.com/hashicorp/terraform-aws-consul-lambda/pull/54)

BUG FIXES
* Security:
    * Upgrade to use Go 1.20.1 This resolves vulnerabilities [CVE-2022-41724](https://go.dev/issue/58001) in `crypto/tls` and [CVE-2022-41723](https://go.dev/issue/57855) in `net/http`. [[GH-1908]](https://github.com/hashicorp/terraform-aws-consul-lambda/pull/54)

## 0.1.0-beta2 (October 04, 2022)

FEATURES
* Add support to enable AWS Lambda functions to call Consul mesh services.

## 0.1.0-beta1 (June 15, 2022)

FEATURES
* Initial release to enable Consul mesh services to invoke AWS Lambda functions.
