#!/usr/bin/env bash
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
NAME_PREFIX="${NAME_PREFIX:-agenthook-prod}"
INSTANCE_TYPE="${INSTANCE_TYPE:-t3.micro}"
APP_PORT="${APP_PORT:-8080}"
APP_ENV_SSM_PARAM="${APP_ENV_SSM_PARAM:-/agenthook/prod/env}"
REPO_URL="${REPO_URL:-https://github.com/abhinaviitg18/webhook_listener.git}"
KEY_NAME="${KEY_NAME:-}"

need() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1"; exit 1; }
}
need aws
need jq

echo "==> discovering default VPC and subnets"
VPC_ID=$(aws ec2 describe-vpcs --region "$AWS_REGION" \
  --filters Name=isDefault,Values=true \
  --query 'Vpcs[0].VpcId' --output text)
if [[ -z "$VPC_ID" || "$VPC_ID" == "None" ]]; then
  echo "default VPC not found in region $AWS_REGION"
  exit 1
fi

SUBNETS=($(aws ec2 describe-subnets --region "$AWS_REGION" \
  --filters Name=vpc-id,Values="$VPC_ID" Name=default-for-az,Values=true \
  --query 'Subnets[].SubnetId' --output text))
if [[ ${#SUBNETS[@]} -lt 2 ]]; then
  echo "need at least 2 default subnets for ALB"
  exit 1
fi

echo "==> creating security groups"
ALB_SG_ID=$(aws ec2 create-security-group --region "$AWS_REGION" \
  --group-name "${NAME_PREFIX}-alb-sg" \
  --description "ALB SG for ${NAME_PREFIX}" \
  --vpc-id "$VPC_ID" \
  --query 'GroupId' --output text 2>/dev/null || true)
if [[ -z "$ALB_SG_ID" ]]; then
  ALB_SG_ID=$(aws ec2 describe-security-groups --region "$AWS_REGION" \
    --filters Name=group-name,Values="${NAME_PREFIX}-alb-sg" Name=vpc-id,Values="$VPC_ID" \
    --query 'SecurityGroups[0].GroupId' --output text)
fi
aws ec2 authorize-security-group-ingress --region "$AWS_REGION" --group-id "$ALB_SG_ID" \
  --ip-permissions '[{"IpProtocol":"tcp","FromPort":80,"ToPort":80,"IpRanges":[{"CidrIp":"0.0.0.0/0"}]}]' >/dev/null 2>&1 || true

EC2_SG_ID=$(aws ec2 create-security-group --region "$AWS_REGION" \
  --group-name "${NAME_PREFIX}-ec2-sg" \
  --description "EC2 SG for ${NAME_PREFIX}" \
  --vpc-id "$VPC_ID" \
  --query 'GroupId' --output text 2>/dev/null || true)
if [[ -z "$EC2_SG_ID" ]]; then
  EC2_SG_ID=$(aws ec2 describe-security-groups --region "$AWS_REGION" \
    --filters Name=group-name,Values="${NAME_PREFIX}-ec2-sg" Name=vpc-id,Values="$VPC_ID" \
    --query 'SecurityGroups[0].GroupId' --output text)
fi
aws ec2 authorize-security-group-ingress --region "$AWS_REGION" --group-id "$EC2_SG_ID" \
  --ip-permissions "[{\"IpProtocol\":\"tcp\",\"FromPort\":${APP_PORT},\"ToPort\":${APP_PORT},\"UserIdGroupPairs\":[{\"GroupId\":\"${ALB_SG_ID}\"}]}]" >/dev/null 2>&1 || true
aws ec2 authorize-security-group-ingress --region "$AWS_REGION" --group-id "$EC2_SG_ID" \
  --ip-permissions '[{"IpProtocol":"tcp","FromPort":22,"ToPort":22,"IpRanges":[{"CidrIp":"0.0.0.0/0"}]}]' >/dev/null 2>&1 || true

echo "==> creating instance role/profile"
ROLE_NAME="${NAME_PREFIX}-ec2-role"
PROFILE_NAME="${NAME_PREFIX}-ec2-profile"
TRUST_DOC=$(mktemp)
cat >"$TRUST_DOC" <<'JSON'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": { "Service": "ec2.amazonaws.com" },
      "Action": "sts:AssumeRole"
    }
  ]
}
JSON

aws iam create-role --role-name "$ROLE_NAME" \
  --assume-role-policy-document "file://${TRUST_DOC}" >/dev/null 2>&1 || true
aws iam attach-role-policy --role-name "$ROLE_NAME" \
  --policy-arn arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore >/dev/null 2>&1 || true

INLINE_POLICY=$(mktemp)
cat >"$INLINE_POLICY" <<JSON
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ReadHookwebEnv",
      "Effect": "Allow",
      "Action": ["ssm:GetParameter"],
      "Resource": "arn:aws:ssm:${AWS_REGION}:*:parameter${APP_ENV_SSM_PARAM}"
    }
  ]
}
JSON
aws iam put-role-policy --role-name "$ROLE_NAME" --policy-name "${NAME_PREFIX}-ssm-env-read" \
  --policy-document "file://${INLINE_POLICY}"

aws iam create-instance-profile --instance-profile-name "$PROFILE_NAME" >/dev/null 2>&1 || true
aws iam add-role-to-instance-profile --instance-profile-name "$PROFILE_NAME" --role-name "$ROLE_NAME" >/dev/null 2>&1 || true
sleep 10

echo "==> uploading app env to parameter store"
if [[ ! -f .env ]]; then
  echo ".env is required in current directory to create SSM parameter ${APP_ENV_SSM_PARAM}"
  exit 1
fi
aws ssm put-parameter --region "$AWS_REGION" \
  --name "$APP_ENV_SSM_PARAM" \
  --type SecureString \
  --overwrite \
  --value "$(cat .env)" >/dev/null

echo "==> finding latest Amazon Linux 2023 AMI"
AMI_ID=$(aws ssm get-parameter --region "$AWS_REGION" \
  --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64 \
  --query 'Parameter.Value' --output text)

USER_DATA=$(mktemp)
cat >"$USER_DATA" <<EOF
#!/bin/bash
set -euo pipefail
dnf update -y
dnf install -y git golang
mkdir -p /opt/agenthook
chown -R ec2-user:ec2-user /opt/agenthook
if [[ ! -d /opt/agenthook/repo/.git ]]; then
  sudo -u ec2-user git clone ${REPO_URL} /opt/agenthook/repo
fi
echo "APP_ENV_SSM_PARAM=${APP_ENV_SSM_PARAM}" >/etc/default/agenthook
EOF

echo "==> launching EC2 instance"
RUN_ARGS=(
  --region "$AWS_REGION"
  --image-id "$AMI_ID"
  --instance-type "$INSTANCE_TYPE"
  --iam-instance-profile Name="$PROFILE_NAME"
  --security-group-ids "$EC2_SG_ID"
  --subnet-id "${SUBNETS[0]}"
  --associate-public-ip-address
  --user-data "file://${USER_DATA}"
  --tag-specifications "ResourceType=instance,Tags=[{Key=Name,Value=${NAME_PREFIX}-ec2}]"
)
if [[ -n "$KEY_NAME" ]]; then
  RUN_ARGS+=(--key-name "$KEY_NAME")
fi
INSTANCE_ID=$(aws ec2 run-instances "${RUN_ARGS[@]}" --query 'Instances[0].InstanceId' --output text)
aws ec2 wait instance-running --region "$AWS_REGION" --instance-ids "$INSTANCE_ID"

echo "==> creating target group and ALB"
TG_ARN=$(aws elbv2 create-target-group --region "$AWS_REGION" \
  --name "${NAME_PREFIX}-tg" \
  --protocol HTTP \
  --port "$APP_PORT" \
  --target-type instance \
  --vpc-id "$VPC_ID" \
  --health-check-path "/" \
  --query 'TargetGroups[0].TargetGroupArn' --output text 2>/dev/null || true)
if [[ -z "$TG_ARN" ]]; then
  TG_ARN=$(aws elbv2 describe-target-groups --region "$AWS_REGION" \
    --names "${NAME_PREFIX}-tg" --query 'TargetGroups[0].TargetGroupArn' --output text)
fi
aws elbv2 register-targets --region "$AWS_REGION" --target-group-arn "$TG_ARN" \
  --targets "Id=${INSTANCE_ID},Port=${APP_PORT}"

ALB_ARN=$(aws elbv2 create-load-balancer --region "$AWS_REGION" \
  --name "${NAME_PREFIX}-alb" \
  --subnets "${SUBNETS[0]}" "${SUBNETS[1]}" \
  --security-groups "$ALB_SG_ID" \
  --type application \
  --scheme internet-facing \
  --query 'LoadBalancers[0].LoadBalancerArn' --output text 2>/dev/null || true)
if [[ -z "$ALB_ARN" ]]; then
  ALB_ARN=$(aws elbv2 describe-load-balancers --region "$AWS_REGION" \
    --names "${NAME_PREFIX}-alb" --query 'LoadBalancers[0].LoadBalancerArn' --output text)
fi

aws elbv2 create-listener --region "$AWS_REGION" \
  --load-balancer-arn "$ALB_ARN" \
  --protocol HTTP \
  --port 80 \
  --default-actions "Type=forward,TargetGroupArn=${TG_ARN}" >/dev/null 2>&1 || true

ALB_DNS=$(aws elbv2 describe-load-balancers --region "$AWS_REGION" \
  --load-balancer-arns "$ALB_ARN" \
  --query 'LoadBalancers[0].DNSName' --output text)

echo
echo "Provisioned resources:"
echo "AWS_REGION=$AWS_REGION"
echo "INSTANCE_ID=$INSTANCE_ID"
echo "EC2_SG_ID=$EC2_SG_ID"
echo "ALB_SG_ID=$ALB_SG_ID"
echo "TARGET_GROUP_ARN=$TG_ARN"
echo "ALB_ARN=$ALB_ARN"
echo "ALB_DNS=$ALB_DNS"
echo "APP_ENV_SSM_PARAM=$APP_ENV_SSM_PARAM"
echo
echo "Next step:"
echo "  aws ssm send-command --region $AWS_REGION --instance-ids $INSTANCE_ID --document-name AWS-RunShellScript --parameters commands='[\"cd /opt/agenthook/repo\",\"sudo APP_ENV_SSM_PARAM=$APP_ENV_SSM_PARAM bash scripts/aws/ec2_deploy.sh\"]'"
