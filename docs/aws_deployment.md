# WebhookListener AWS Deployment Guide

This document outlines the post-deployment steps after running the `scripts/aws/deploy.py` script. 

The deployment script provisions:
1.  **IAM Execution Role** with `ssm:GetParameter` access.
2.  **AWS Lambda Function** configured for `provided.al2023`.
3.  **Lambda Function URL** (publicly accessible).

## 1. Updating the Lambda Function Code

The deployment script creates the infrastructure but contains a dummy lambda function. You must compile the Go binary and deploy it to the Lambda function.

### Compilation
Build the Go binary for the `provided.al2023` runtime (Amazon Linux 2023 requires `linux/arm64` or `linux/amd64`, depending on your Lambda architecture, defaulting to `x86_64`):

```bash
GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o bootstrap cmd/lambda/main.go
zip deployment.zip bootstrap
```

### Deployment
Upload the `deployment.zip` to your Lambda function:

```bash
aws lambda update-function-code \
  --function-name WebhookListenerFunction \
  --zip-file fileb://deployment.zip
```

> [!NOTE]
> Make sure `APP_ENV_SSM_PARAM` is correctly set in the SSM Parameter Store (`/agenthook/prod/env`) as a `SecureString` or `String` containing the `.env` content.

---

## 2. Pointing Cloudflare to Lambda Function URL

To secure your Lambda and provide custom domain routing, put it behind Cloudflare.

1.  **Get the Function URL:** Run the `deploy.py` script or check the AWS Console to retrieve your Lambda Function URL (e.g., `https://<id>.lambda-url.<region>.on.aws/`).
2.  **Cloudflare DNS:** Create a DNS record (e.g., CNAME for `api.agenthook.store`) pointing to the Lambda Function URL. Ensure the Cloudflare proxy (orange cloud) is **ON**.
3.  **Cloudflare Workers/Rules (Optional but Recommended):** 
    You should enforce that the Lambda only accepts traffic from Cloudflare by setting the `LAMBDA_ORIGIN_SHARED_SECRET` environment variable in your Lambda and injecting `x-agenthook-origin-secret` via a Cloudflare Transform Rule.
    
    *   **In AWS Lambda:** Set `LAMBDA_ORIGIN_SHARED_SECRET` to a secure random string (e.g., `super-secret-123`).
    *   **In Cloudflare:** Go to **Rules > Transform Rules > Modify Request Header**.
    *   **Rule Configuration:** 
        *   If URI Path starts with `/*`
        *   Set static header `x-agenthook-origin-secret` to `super-secret-123`.

> [!IMPORTANT]
> If you don't use the shared secret, your Function URL is open to the public internet and anyone can bypass Cloudflare's WAF.

---

## 3. Database Connection & VPC Configuration

By default, the Lambda is deployed **outside a VPC**. This is the recommended approach if you are using:
- TiDB Serverless
- PlanetScale
- Supabase
- DynamoDB / Pinecone

### If you have an internal Database (RDS / Aurora / Private VPC)

If your database is situated inside an AWS VPC and is not publicly accessible, you **must** configure the Lambda to connect to that VPC.

1.  **Update IAM Permissions:** 
    Ensure the Lambda Execution Role has the `AWSLambdaVPCAccessExecutionRole` managed policy. (You can uncomment this line in the `CFN_TEMPLATE` inside `scripts/aws/deploy.py`).
2.  **Update Lambda VPC Settings:**
    Attach the Lambda to the VPC via the AWS Console or CLI:
    ```bash
    aws lambda update-function-configuration \
      --function-name WebhookListenerFunction \
      --vpc-config SubnetIds=subnet-123,subnet-456,SecurityGroupIds=sg-789
    ```
3.  **Connection Pooling (RDS Proxy):**
    If using RDS or Aurora, heavily consider using **Amazon RDS Proxy**. Lambdas scale rapidly and can exhaust database connections quickly. RDS Proxy maintains a pool of connections and multiplexes them.
4.  **NAT Gateway:**
    > [!WARNING]  
    > Once a Lambda is attached to a private subnet in a VPC, it loses direct internet access. Because WebhookListener requires internet access to call LLM Providers (OpenRouter, Groq) and Pinecone, you **MUST** have a NAT Gateway configured in the VPC routing table for those subnets.
