#!/usr/bin/env python3
import argparse
import sys
import time

try:
    import boto3
    from botocore.exceptions import ClientError
except ImportError:
    print("Error: boto3 is not installed. Please run `pip install boto3`")
    sys.exit(1)

# The CloudFormation Template
CFN_TEMPLATE = """
AWSTemplateFormatVersion: '2010-09-09'
Description: 'WebhookListener Lambda Deployment'

Parameters:
  FunctionName:
    Type: String
    Default: 'WebhookListenerFunction'
    Description: 'The name of the Lambda function'
  SSMParameterName:
    Type: String
    Default: '/agenthook/prod/env'
    Description: 'The SSM Parameter Store path containing the .env file contents'

Resources:
  # IAM Role for the Lambda Function
  LambdaExecutionRole:
    Type: AWS::IAM::Role
    Properties:
      RoleName: !Sub '${FunctionName}-ExecutionRole'
      AssumeRolePolicyDocument:
        Version: '2012-10-17'
        Statement:
          - Effect: Allow
            Principal:
              Service: lambda.amazonaws.com
            Action: sts:AssumeRole
      ManagedPolicyArns:
        - arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole
        # Uncomment the following line if you decide to deploy the Lambda in a VPC
        # - arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole
      Policies:
        - PolicyName: ReadSSMEnv
          PolicyDocument:
            Version: '2012-10-17'
            Statement:
              - Effect: Allow
                Action:
                  - ssm:GetParameter
                Resource: !Sub 'arn:aws:ssm:${AWS::Region}:${AWS::AccountId}:parameter${SSMParameterName}'

  # The Lambda Function itself
  WebhookListenerLambda:
    Type: AWS::Lambda::Function
    Properties:
      FunctionName: !Ref FunctionName
      Runtime: provided.al2023
      Handler: bootstrap
      Role: !GetAtt LambdaExecutionRole.Arn
      # Dummy code block to allow creation without immediate S3 artifact.
      # You should update this function code using AWS CLI or CI/CD after stack creation.
      Code:
        ZipFile: |
          #!/bin/bash
          echo "Please update the lambda code with your Go binary."
      Timeout: 30
      MemorySize: 256
      Environment:
        Variables:
          APP_ENV_SSM_PARAM: !Ref SSMParameterName
          LAMBDA_ORIGIN_SHARED_SECRET: '' # Can be passed in via CI/CD later
      Tags:
        - Key: Name
          Value: WebOklizner
        - Key: Project
          Value: WebOklizner

  # Function URL to expose the Lambda to Cloudflare
  LambdaFunctionUrl:
    Type: AWS::Lambda::Url
    Properties:
      TargetFunctionArn: !GetAtt WebhookListenerLambda.Arn
      AuthType: NONE

  # Allow public access to the Function URL
  FunctionUrlResourcePolicy:
    Type: AWS::Lambda::Permission
    Properties:
      FunctionName: !Ref WebhookListenerLambda
      Action: lambda:InvokeFunctionUrl
      Principal: '*'
      FunctionUrlAuthType: NONE

Outputs:
  FunctionUrl:
    Description: 'URL to invoke the Lambda function'
    Value: !GetAtt LambdaFunctionUrl.FunctionUrl
  ExecutionRoleArn:
    Description: 'ARN of the Lambda Execution Role'
    Value: !GetAtt LambdaExecutionRole.Arn
"""

def print_environment_requirements():
    print("="*60)
    print("ENVIRONMENT VARIABLE REQUIREMENTS & EXPLANATION")
    print("="*60)
    print("Before deploying, ensure you have set up an SSM Parameter (default: /agenthook/prod/env)")
    print("containing your .env configuration. The Go application requires the following key variables:\n")
    print("  1. TIDB_DSN / Database URL:")
    print("     - Why: Required for persisting webhook configurations, routing rules, and history.")
    print("  2. SCALEKIT_* variables:")
    print("     - Why: Required for SSO, authentication, and tenant management.")
    print("  3. PINECONE_* variables:")
    print("     - Why: Required for semantic search and vector database functionalities (RAG).")
    print("  4. LLM Providers (OPENROUTER_*, GROQ_*, CEREBRAS_*):")
    print("     - Why: Required for evaluating incoming webhooks, categorizing payloads, and routing.")
    print("  5. APP_SESSION_SECRET & LAMBDA_ORIGIN_SHARED_SECRET:")
    print("     - Why: Required for secure cookie sessions and validating traffic from Cloudflare.")
    print("\nIf you haven't set these up in SSM Parameter Store, the Lambda will fail to initialize.")
    print("="*60)

def deploy_stack(stack_name, region_name):
    print(f"\nInitializing deployment for stack: {stack_name} in region: {region_name}")
    
    session = boto3.Session(region_name=region_name)
    cfn = session.client('cloudformation')

    tags = [
        {'Key': 'Name', 'Value': 'WebOklizner'},
        {'Key': 'Project', 'Value': 'WebOklizner'}
    ]

    try:
        cfn.describe_stacks(StackName=stack_name)
        stack_exists = True
    except ClientError as e:
        if 'does not exist' in str(e):
            stack_exists = False
        else:
            raise

    try:
        if stack_exists:
            print(f"Updating existing stack: {stack_name}...")
            cfn.update_stack(
                StackName=stack_name,
                TemplateBody=CFN_TEMPLATE,
                Capabilities=['CAPABILITY_NAMED_IAM'],
                Tags=tags
            )
            waiter = cfn.get_waiter('stack_update_complete')
            print("Waiting for update to complete...")
            waiter.wait(StackName=stack_name)
            print("Stack updated successfully!")
        else:
            print(f"Creating new stack: {stack_name}...")
            cfn.create_stack(
                StackName=stack_name,
                TemplateBody=CFN_TEMPLATE,
                Capabilities=['CAPABILITY_NAMED_IAM'],
                Tags=tags
            )
            waiter = cfn.get_waiter('stack_create_complete')
            print("Waiting for creation to complete...")
            waiter.wait(StackName=stack_name)
            print("Stack created successfully!")

        # Fetch outputs
        response = cfn.describe_stacks(StackName=stack_name)
        outputs = response['Stacks'][0].get('Outputs', [])
        
        print("\n" + "="*60)
        print("DEPLOYMENT OUTPUTS")
        print("="*60)
        for output in outputs:
            print(f"{output['OutputKey']}: {output['OutputValue']}")
        print("="*60)
        
    except ClientError as e:
        if 'No updates are to be performed' in str(e):
            print("\nNo updates were necessary. Stack is already up-to-date.")
            
            # Still fetch outputs
            response = cfn.describe_stacks(StackName=stack_name)
            outputs = response['Stacks'][0].get('Outputs', [])
            print("\n" + "="*60)
            print("DEPLOYMENT OUTPUTS")
            print("="*60)
            for output in outputs:
                print(f"{output['OutputKey']}: {output['OutputValue']}")
            print("="*60)
        else:
            print(f"\nDeployment failed: {str(e)}")
            sys.exit(1)

def main():
    parser = argparse.ArgumentParser(description="Deploy WebhookListener to AWS via CloudFormation")
    parser.add_argument('--stack-name', default='WebhookListenerStack', help='Name of the CloudFormation stack')
    parser.add_argument('--region', default='us-east-1', help='AWS Region to deploy to')
    parser.add_argument('--yes', '-y', action='store_true', help='Skip confirmation prompt')
    args = parser.parse_args()

    print_environment_requirements()
    
    if not args.yes:
        response = input("\nHave you prepared the SSM parameter and do you want to proceed with deployment? (y/N): ")
        if response.lower() not in ['y', 'yes']:
            print("Deployment cancelled.")
            sys.exit(0)
            
    deploy_stack(args.stack_name, args.region)

if __name__ == "__main__":
    main()
