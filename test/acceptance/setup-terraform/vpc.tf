module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "2.78.0"

  name = local.name
  cidr = "10.0.0.0/16"
  azs = [
    // Silly, but avoids this error: `"count" value depends on resource attributes that cannot be determined until apply`
    random_shuffle.azs.result[0],
    random_shuffle.azs.result[1],
    random_shuffle.azs.result[2],
  ]
  private_subnets      = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  public_subnets       = ["10.0.4.0/24", "10.0.5.0/24", "10.0.6.0/24"]
  enable_nat_gateway   = true
  single_nat_gateway   = true
  enable_dns_hostnames = true
}
