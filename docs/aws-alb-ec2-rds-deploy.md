# AWS ALB + EC2 + RDS Deployment Runbook

This project now includes scripts and CI for:
- Creating an internet-facing ALB.
- Launching a `t3.micro` EC2 instance.
- Registering EC2 in an ALB target group.
- Deploying app updates from GitHub Actions via SSM.
- Applying schema to an existing RDS MySQL-compatible instance.

## 1) Prerequisites

- AWS credentials with permissions for EC2/ELBv2/IAM/SSM/RDS/SecretsManager.
- `aws`, `jq`, `mysql`, and `gh` CLIs installed locally.
- `.env` present in repo root (used to seed SSM env parameter).

## 2) Provision ALB + EC2

From repo root:

```bash
chmod +x scripts/aws/*.sh
AWS_REGION=us-east-1 \
NAME_PREFIX=agenthook-prod \
APP_ENV_SSM_PARAM=/agenthook/prod/env \
REPO_URL=https://github.com/abhinaviitg18/webhook_listener.git \
./scripts/aws/provision_alb_ec2.sh
```

This prints:
- `INSTANCE_ID`
- `ALB_DNS`
- `APP_ENV_SSM_PARAM`

## 3) Apply schema in existing RDS

Use direct DB credentials:

```bash
RDS_HOST=<existing-rds-endpoint> \
RDS_PORT=3306 \
RDS_USER=<user> \
RDS_PASSWORD=<password> \
DB_NAME=agenthook \
./scripts/aws/apply_rds_schema.sh
```

Or use a Secrets Manager secret that stores `host`, `port`, `username`, `password`, `dbname`:

```bash
AWS_REGION=us-east-1 \
RDS_SECRET_ARN=arn:aws:secretsmanager:...:secret:... \
./scripts/aws/apply_rds_schema.sh
```

## 4) Configure GitHub Actions environment secrets

Set `EC2_INSTANCE_ID` from provisioning output:

```bash
EC2_INSTANCE_ID=i-xxxxxxxxxxxxxxxxx \
REPO=abhinaviitg18/webhook_listener \
ENV_NAME=production \
./scripts/aws/setup_github_secrets.sh
```

Required environment secrets for workflow:
- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`
- `AWS_REGION`
- `EC2_INSTANCE_ID`
- `APP_ENV_SSM_PARAM`

## 5) Deploy

Workflow file:
- `.github/workflows/deploy-aws-ec2.yml`

Deploy triggers:
- Push to `master`
- Manual `workflow_dispatch`

The deploy job runs tests, then executes `scripts/aws/ec2_deploy.sh` on EC2 through SSM.

## 6) Verify

- ALB health: `http://<ALB_DNS>/`
- Service status on EC2:

```bash
aws ssm send-command \
  --instance-ids <INSTANCE_ID> \
  --document-name AWS-RunShellScript \
  --parameters commands='["sudo systemctl status agenthook --no-pager","curl -sS http://127.0.0.1:8080/ | head -c 200"]'
```

## 7) IAM permissions needed

The provisioning IAM principal must allow at least:
- `ec2:DescribeVpcs`, `ec2:DescribeSubnets`, `ec2:DescribeImages`, `ec2:RunInstances`, `ec2:CreateSecurityGroup`, `ec2:AuthorizeSecurityGroupIngress`, `ec2:DescribeSecurityGroups`, `ec2:CreateTags`, `ec2:DescribeInstances`
- `elasticloadbalancing:CreateLoadBalancer`, `elasticloadbalancing:CreateTargetGroup`, `elasticloadbalancing:RegisterTargets`, `elasticloadbalancing:CreateListener`, `elasticloadbalancing:Describe*`
- `iam:CreateRole`, `iam:AttachRolePolicy`, `iam:PutRolePolicy`, `iam:CreateInstanceProfile`, `iam:AddRoleToInstanceProfile`, `iam:GetRole`, `iam:GetInstanceProfile`
- `ssm:PutParameter`, `ssm:SendCommand`, `ssm:GetCommandInvocation`, `ssm:DescribeInstanceInformation`, `ssm:Wait*`
- `rds:DescribeDBInstances` (for discovery)
- `secretsmanager:GetSecretValue` (if using `RDS_SECRET_ARN`)
