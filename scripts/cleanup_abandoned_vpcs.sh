#!/usr/bin/env bash
set -euo pipefail

# Finds likely abandoned VPCs and optionally deletes them.
# Default mode is dry-run. Deletion requires --apply.
#
# Usage examples:
#   ./scripts/cleanup_abandoned_vpcs.sh --profile default --region us-west-2
#   ./scripts/cleanup_abandoned_vpcs.sh --profile default --region us-west-2 --apply --delete-empty-sgs --delete-nacls
#   ./scripts/cleanup_abandoned_vpcs.sh --region us-west-2 --apply --name-regex '^(lambda-registrator-|consul-ecs-)'

PROFILE="${AWS_PROFILE:-}"
REGION="${AWS_REGION:-${AWS_DEFAULT_REGION:-us-west-2}}"
APPLY="false"
DELETE_EMPTY_SGS="false"
DELETE_NACLS="false"
NAME_REGEX=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile)
      PROFILE="$2"
      shift 2
      ;;
    --region)
      REGION="$2"
      shift 2
      ;;
    --apply)
      APPLY="true"
      shift
      ;;
    --delete-empty-sgs)
      DELETE_EMPTY_SGS="true"
      shift
      ;;
    --delete-nacls)
      DELETE_NACLS="true"
      shift
      ;;
    --name-regex)
      NAME_REGEX="$2"
      shift 2
      ;;
    -h|--help)
      sed -n '1,28p' "$0"
      exit 0
      ;;
    *)
      echo "Unknown arg: $1"
      exit 1
      ;;
  esac
done

AWS=(aws --region "$REGION")
if [[ -n "$PROFILE" ]]; then
  AWS+=(--profile "$PROFILE")
fi

if [[ -n "$PROFILE" ]]; then
  echo "Checking AWS identity for profile=$PROFILE region=$REGION"
else
  echo "Checking AWS identity using env/default credentials region=$REGION"
fi
"${AWS[@]}" sts get-caller-identity >/dev/null

echo "Collecting VPC inventory..."
VPCS=$("${AWS[@]}" ec2 describe-vpcs \
  --filters Name=is-default,Values=false \
  --query 'Vpcs[].VpcId' \
  --output text)

if [[ -z "${VPCS// }" ]]; then
  echo "No non-default VPCs found in $REGION."
  exit 0
fi

has_instances() {
  local vpc_id="$1"
  local n
  n=$("${AWS[@]}" ec2 describe-instances \
    --filters Name=vpc-id,Values="$vpc_id" Name=instance-state-name,Values=pending,running,stopping,stopped \
    --query 'length(Reservations[])' --output text)
  [[ "$n" != "0" ]]
}

has_eni() {
  local vpc_id="$1"
  local n
  n=$("${AWS[@]}" ec2 describe-network-interfaces \
    --filters Name=vpc-id,Values="$vpc_id" Name=status,Values=in-use \
    --query 'length(NetworkInterfaces[])' --output text)
  [[ "$n" != "0" ]]
}

has_nat() {
  local vpc_id="$1"
  local n
  n=$("${AWS[@]}" ec2 describe-nat-gateways \
    --filter Name=vpc-id,Values="$vpc_id" \
    --query "length(NatGateways[?State!='deleted'])" --output text)
  [[ "$n" != "0" ]]
}

has_vpce() {
  local vpc_id="$1"
  local n
  n=$("${AWS[@]}" ec2 describe-vpc-endpoints \
    --filters Name=vpc-id,Values="$vpc_id" \
    --query 'length(VpcEndpoints[])' --output text)
  [[ "$n" != "0" ]]
}

has_tgw_attachment() {
  local vpc_id="$1"
  local n
  n=$("${AWS[@]}" ec2 describe-transit-gateway-vpc-attachments \
    --filters Name=vpc-id,Values="$vpc_id" \
    --query "length(TransitGatewayVpcAttachments[?State!='deleted'])" --output text 2>/dev/null || echo 0)
  [[ "$n" != "0" ]]
}

has_lb() {
  local vpc_id="$1"
  local n1 n2
  n1=$("${AWS[@]}" elbv2 describe-load-balancers \
    --query "length(LoadBalancers[?VpcId=='$vpc_id'])" --output text 2>/dev/null || echo 0)
  n2=$("${AWS[@]}" elb describe-load-balancers \
    --query "length(LoadBalancerDescriptions[?VPCId=='$vpc_id'])" --output text 2>/dev/null || echo 0)
  [[ "$n1" != "0" || "$n2" != "0" ]]
}

is_candidate_abandoned() {
  local vpc_id="$1"
  if has_instances "$vpc_id"; then return 1; fi
  if has_eni "$vpc_id"; then return 1; fi
  if has_nat "$vpc_id"; then return 1; fi
  if has_vpce "$vpc_id"; then return 1; fi
  if has_tgw_attachment "$vpc_id"; then return 1; fi
  if has_lb "$vpc_id"; then return 1; fi
  return 0
}

cleanup_and_delete_vpc() {
  local vpc_id="$1"
  echo "Deleting dependencies for $vpc_id"

  # Detach + delete IGW
  local igw_ids
  igw_ids=$("${AWS[@]}" ec2 describe-internet-gateways \
    --filters Name=attachment.vpc-id,Values="$vpc_id" \
    --query 'InternetGateways[].InternetGatewayId' --output text)
  for igw in $igw_ids; do
    echo "  Detach/Delete IGW: $igw"
    "${AWS[@]}" ec2 detach-internet-gateway --internet-gateway-id "$igw" --vpc-id "$vpc_id" || true
    "${AWS[@]}" ec2 delete-internet-gateway --internet-gateway-id "$igw" || true
  done

  # Delete subnets
  local subnet_ids
  subnet_ids=$("${AWS[@]}" ec2 describe-subnets --filters Name=vpc-id,Values="$vpc_id" --query 'Subnets[].SubnetId' --output text)
  for subnet in $subnet_ids; do
    echo "  Delete subnet: $subnet"
    "${AWS[@]}" ec2 delete-subnet --subnet-id "$subnet" || true
  done

  # Delete non-main route tables after subnets are removed.
  # Some route tables remain associated to subnets until subnet deletion completes.
  local rtb_ids
  rtb_ids=$("${AWS[@]}" ec2 describe-route-tables \
    --filters Name=vpc-id,Values="$vpc_id" \
    --query 'RouteTables[?Associations[?Main!=`true`]].RouteTableId' --output text)
  for rtb in $rtb_ids; do
    local assoc_ids
    assoc_ids=$("${AWS[@]}" ec2 describe-route-tables --route-table-ids "$rtb" --query 'RouteTables[0].Associations[?Main!=`true`].RouteTableAssociationId' --output text)
    for assoc in $assoc_ids; do
      if [[ -n "$assoc" && "$assoc" != "None" ]]; then
        echo "  Disassociate route table association: $assoc"
        "${AWS[@]}" ec2 disassociate-route-table --association-id "$assoc" || true
      fi
    done
    echo "  Delete route table: $rtb"
    "${AWS[@]}" ec2 delete-route-table --route-table-id "$rtb" || true
  done

  # Optionally delete custom SGs with no ENIs attached
  if [[ "$DELETE_EMPTY_SGS" == "true" ]]; then
    local sg_ids
    sg_ids=$("${AWS[@]}" ec2 describe-security-groups \
      --filters Name=vpc-id,Values="$vpc_id" \
      --query "SecurityGroups[?GroupName!='default'].GroupId" --output text)
    for sg in $sg_ids; do
      local eni_count
      eni_count=$("${AWS[@]}" ec2 describe-network-interfaces \
        --filters Name=group-id,Values="$sg" \
        --query 'length(NetworkInterfaces[])' --output text)
      if [[ "$eni_count" == "0" ]]; then
        local ingress_rules
        ingress_rules=$("${AWS[@]}" ec2 describe-security-groups --group-ids "$sg" --query 'SecurityGroups[0].IpPermissions' --output json)
        if [[ "$ingress_rules" != "[]" ]]; then
          echo "  Revoke ingress rules for security group: $sg"
          "${AWS[@]}" ec2 revoke-security-group-ingress --group-id "$sg" --ip-permissions "$ingress_rules" || true
        fi

        local egress_rules
        egress_rules=$("${AWS[@]}" ec2 describe-security-groups --group-ids "$sg" --query 'SecurityGroups[0].IpPermissionsEgress' --output json)
        if [[ "$egress_rules" != "[]" ]]; then
          echo "  Revoke egress rules for security group: $sg"
          "${AWS[@]}" ec2 revoke-security-group-egress --group-id "$sg" --ip-permissions "$egress_rules" || true
        fi

        echo "  Delete security group: $sg"
        "${AWS[@]}" ec2 delete-security-group --group-id "$sg" || true
      fi
    done
  fi

  # Optionally delete custom NACLs
  if [[ "$DELETE_NACLS" == "true" ]]; then
    local default_nacl
    default_nacl=$("${AWS[@]}" ec2 describe-network-acls \
      --filters Name=vpc-id,Values="$vpc_id" Name=default,Values=true \
      --query 'NetworkAcls[0].NetworkAclId' --output text)

    local nacl_ids
    nacl_ids=$("${AWS[@]}" ec2 describe-network-acls \
      --filters Name=vpc-id,Values="$vpc_id" \
      --query 'NetworkAcls[?IsDefault==`false`].NetworkAclId' --output text)
    for nacl in $nacl_ids; do
      local assoc_ids
      assoc_ids=$("${AWS[@]}" ec2 describe-network-acls --network-acl-ids "$nacl" --query 'NetworkAcls[0].Associations[].NetworkAclAssociationId' --output text)
      for assoc in $assoc_ids; do
        if [[ -n "$assoc" && "$assoc" != "None" && -n "$default_nacl" && "$default_nacl" != "None" ]]; then
          echo "  Reassociate NACL association $assoc to default NACL: $default_nacl"
          "${AWS[@]}" ec2 replace-network-acl-association --association-id "$assoc" --network-acl-id "$default_nacl" >/dev/null 2>&1 || true
        fi
      done

      echo "  Delete NACL: $nacl"
      "${AWS[@]}" ec2 delete-network-acl --network-acl-id "$nacl" || true
    done
  fi

  echo "  Delete VPC: $vpc_id"
  local i
  for i in 1 2 3; do
    if "${AWS[@]}" ec2 delete-vpc --vpc-id "$vpc_id"; then
      echo "  Deleted VPC: $vpc_id"
      return 0
    fi
    echo "  Retry delete VPC attempt $i failed for $vpc_id; waiting before retry..."
    sleep 3
  done

  echo "  Failed to delete VPC after retries: $vpc_id"
  return 1
}

echo "\n=== Candidate abandoned VPCs (safe checks) ==="
CANDIDATES=()
for vpc in $VPCS; do
  if is_candidate_abandoned "$vpc"; then
    name=$("${AWS[@]}" ec2 describe-tags --filters Name=resource-id,Values="$vpc" Name=key,Values=Name --query 'Tags[0].Value' --output text 2>/dev/null || true)
    if [[ -n "$NAME_REGEX" ]]; then
      if [[ -z "$name" || "$name" == "None" ]]; then
        echo "SKIP      vpc=$vpc name=none (name-regex set: $NAME_REGEX)"
        continue
      fi
      if ! [[ "$name" =~ $NAME_REGEX ]]; then
        echo "SKIP      vpc=$vpc name=$name (name-regex mismatch)"
        continue
      fi
    fi
    cidr=$("${AWS[@]}" ec2 describe-vpcs --vpc-ids "$vpc" --query 'Vpcs[0].CidrBlock' --output text)
    echo "CANDIDATE vpc=$vpc name=${name:-none} cidr=$cidr"
    CANDIDATES+=("$vpc")
  else
    echo "IN-USE    vpc=$vpc"
  fi
done

if [[ "${#CANDIDATES[@]}" -eq 0 ]]; then
  echo "\nNo abandoned VPC candidates found by this script in $REGION."
  exit 0
fi

if [[ "$APPLY" != "true" ]]; then
  echo "\nDry-run complete. No VPCs were deleted."
  echo "To delete candidates, rerun with --apply (and optionally --delete-empty-sgs --delete-nacls)."
  exit 0
fi

echo "\nAPPLY mode enabled. Deleting candidate VPCs..."
for vpc in "${CANDIDATES[@]}"; do
  cleanup_and_delete_vpc "$vpc"
done

echo "Done."
