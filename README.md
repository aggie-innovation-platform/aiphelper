# AIP SSO Helper

This utility helps set up various CLI tools with access to multiple cloud accounts. For AWS it uses AWS SSO to sign you in and gather information about the accounts you have access to.

## Usage

```
$ aip-sso-helper -help
  -accounts string
    	Comma-separated list of accounts to tell Steampipe to connect to (default: all accounts assigned to you through SSO)
  -default-aws-format string
    	Output format for AWS CLI (default "json")
  -default-aws-region string
    	Default region for AWS CLI operations (default "us-east-1")
  -regions string
    	Comma-separated list of regions to tell Steampipe to connect to (default: uses same search order as aws cli)
  -sso-region string
    	AWS SSO Region
  -sso-role-name string
    	SSO Role To Assume (must be the same across all accounts) (default "AdministratorAccess")
  -sso-start-url string
    	AWS SSO Start URL
```

Example usage:

```
# Minimum required parameters

aip-sso-helper -sso-start-url="https://aggie-innovation-platform.awsapps.com/start" -sso-region="us-east-2"
```

## AWS

For AWS, this creates a profile for each AWS account you have access to via SSO so you can scope commands with just a profile name:
```
aws ec2 describe-instances --profile=div_dept_my_account_002
```

Either a normalized account name (all lowercase and underscores) or the account ID can be used as the profile name.

## Steampipe

### AWS

Creates steampipe aws plugin connectors 1:1 with AWS profiles and for each region specified (defaults to the AWS CLI default values). Also created an aggregate connector `aws_all` with one of each AWS account (uses the account ID profile).
