package config

type Parameters struct {
	Aws   AwsParameters   `command:"aws" description:"Initialize AWS"`
	Azure AzureParameters `command:"azure" description:"Initialize Azure"`
}

type AwsParameters struct {
	SSOStartURL   string   `long:"sso-start-url" default:"https://aggie-innovation-platform.awsapps.com/start" description:"AWS SSO Start URL"`
	SSORegion     string   `long:"sso-region" default:"us-east-2" description:"AWS SSO Region"`
	SSORoleName   string   `long:"sso-role-name" default:"AdministratorAccess" description:"SSO Role To Assume (must be the same across all accounts)"`
	Regions       []string `long:"regions" description:"Comma-separated list of regions to tell Steampipe to connect to (default: uses same search order as aws cli)"`
	Accounts      []string `long:"accounts" description:"Comma-separated list of accounts to tell Steampipe to connect to (default: all accounts assigned to you through SSO)"`
	DefaultFormat string   `long:"output-format" default:"json" description:"Output format for AWS CLI"`
	DefaultRegion string   `long:"default-region" default:"us-east-1" description:"Default region for AWS CLI operations"`
}

type AzureParameters struct {
	TenantID            string `long:"tenant-id" default:"68f381e3-46da-47b9-ba57-6f322b8f0da1" description:"Azure Tenant ID"`
	RootManagementGroup string `long:"root-group" short:"g" default:"tamu" description:"management group IDs to begin search for subscriptions"`
	// ExcludeManagementGroups []string `long:"exclude-groups" short:"e" default:"sandbox" description:"comma-separated list of one or more nested management group IDs to exclude"`
}
