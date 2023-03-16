## 0.1.0-beta3 (Mar 16, 2023)

BREAKING CHANGES
* `EnvoyExtensions` configuration was released in Consul 1.15.0 and is now used to configure Lambda functions.
  [[GH-51]](https://github.com/hashicorp/terraform-aws-consul-lambda/pull/51)

FEATURES
* Update minimum go version for project to 1.20 [[GH-1908]](https://github.com/hashicorp/terraform-aws-consul-lambda/pull/54)

BUG FIXES:
* Security:
    * Upgrade to use Go 1.20.1 This resolves vulnerabilities [CVE-2022-41724](https://go.dev/issue/58001) in `crypto/tls` and [CVE-2022-41723](https://go.dev/issue/57855) in `net/http`. [[GH-1908]](https://github.com/hashicorp/terraform-aws-consul-lambda/pull/54)

## 0.1.0-beta2 (October 04, 2022)

FEATURES
* Add support to enable AWS Lambda functions to call Consul mesh services.

## 0.1.0-beta1 (June 14, 2022)

FEATURES
* Initial release to enable Consul mesh services to invoke AWS Lambda functions.
