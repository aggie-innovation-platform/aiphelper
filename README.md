# AIP SSO Helper

This utility helps set up various CLI tools with access to multiple cloud accounts. For AWS it uses AWS SSO to sign you in and gather information about the accounts you have access to.

## Usage

```
Usage:
  main [OPTIONS] <aws | azure>

Application Options:
  -V, --version  aiphelper Version

Help Options:
  -h, --help  Show this help message

Available commands:
  aws    Initialize AWS
  azure  Initialize Azure

[aws command options]
          --sso-start-url=  AWS SSO Start URL (default: https://aggie-innovation-platform.awsapps.com/start)
          --sso-region=     AWS SSO Region (default: us-east-2)
          --sso-role-name=  SSO Role To Assume (must be the same across all accounts) (default: AdministratorAccess)
          --regions=        Comma-separated list of regions to tell Steampipe to connect to (default: uses same search order as aws cli)
          --accounts=       Comma-separated list of accounts to tell Steampipe to connect to (default: all accounts assigned to you through SSO)
          --output-format=  Output format for AWS CLI (default: json)
          --default-region= Default region for AWS CLI operations (default: us-east-1)

[azure command options]
          --tenant-id=       Azure Tenant ID (default: 68f381e3-46da-47b9-ba57-6f322b8f0da1)
      -g, --enum-mgmt-group  Enumerate Azure Management Group descendants for a list of Subscriptions
          --root-group=      management group IDs to begin search for subscriptions (default: tamu)
          --auth-method=     Authentication method to use. Options: [environment, cli, managed-identity, device-code, default] (default: default)
```

Example usage:

```
aiphelper aws # Default regions
aiphelper aws --regions us-east-1,us-west-1 

```

## AWS

`aiphelper` will create an aws profile for each account you have access to based on the account's display name. To use a profile, pass the profile name to the aws cli:

```
aws ec2 describe-instances --profile=div_dept_my_account_002
```

Either a normalized account name (all lowercase and underscores) or the account ID can be used as the profile name.

If you already have an AWS CLI SSO token that matches the SSO URL and region, it will be used. Otherwise, a bew device flow authentication will be started using the SSO parameters, and the token will be cached to disk for further AWS CLI operations.

## Azure

`aiphelper` requires Azure to already be authenticated and by default will use a series of locations to look for credentials: environment variables, a managed identity, or the azure CLI. To learn more, see [DefaultAzureCredential](https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/azidentity#readme-defaultazurecredential).

The easiest way to get started is to use the `az login` command to authenticate the azure CLI with Azure.

If you need to specify an authentication method, such as to use CLI or ENV credentials on an Azure VM with a managed identity, use the `--auth-method` option.

## Steampipe

### AWS

`aiphelper` will create a steampipe connector for each AWS profile and for each region specified (defaults to the AWS CLI default values). This will result in two connectors for each AWS account: `aws_<normalizedname>` and `aws_<accountnumber>`. It will also create an aggregate connector `aws_all` with one of each AWS account, using the `aws_<accountnumber>` connector. 

### Azure

`aiphelper` will create a steampipe connector for each Azure subscription it discovers. It will also create an aggregate connector `azure_all` with every Azure subscription.

### Performance

It is highly recommended to limit the number of connectors and tables being queried to limit the number of API calls steampipe must make. This is especially important for the aggregate connectors. For these, it will be imperative to only fetch the precise columns you need from the tables. Do not fetch all columns if you want your computer to stay calm.
