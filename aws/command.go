package aws

import (
	"errors"

	"github.com/jessevdk/go-flags"
	"github.com/tamu-edu/aiphelper/utils"
)

type Regions struct {
	All []string
}

type Accounts struct {
	All []string
}

var (
	options *Options
)

type Options struct {
	SSOStartURL string   `long:"sso-start-url" default:"https://aggie-innovation-platform.awsapps.com/start" description:"AWS SSO Start URL"`
	SSORegion   string   `long:"sso-region" default:"us-east-2" description:"AWS SSO Region"`
	SSORoleName string   `long:"sso-role-name" default:"AdministratorAccess" description:"SSO Role To Assume (must be the same across all accounts)"`
	Regions     *Regions `long:"regions" default:"" description:"Comma-separated list of regions to tell Steampipe to connect to (default: uses same search order as aws cli)"`
	// Regions       func(r string) []string `long:"regions" description:"Comma-separated list of regions to tell Steampipe to connect to (default: uses same search order as aws cli)"`
	Accounts      *Accounts `long:"accounts" default:"" description:"Comma-separated list of accounts to tell Steampipe to connect to (default: all accounts assigned to you through SSO)"`
	DefaultFormat string    `long:"output-format" default:"json" description:"Output format for AWS CLI"`
	DefaultRegion string    `long:"default-region" default:"us-east-1" description:"Default region for AWS CLI operations"`
}

func AddCommand(p *flags.Parser) {
	options = &Options{}
	// options.Regions = func(r string) []string {
	// 	var tempValue = utils.SplitArgumentParser(r)
	// 	if len(tempValue) == 0 {
	// 		return []string{}
	// 	}
	// 	return tempValue
	// }

	p.AddCommand("aws", "Initialize AWS", "Initialize AWS", options)
}

func (r *Regions) UnmarshalFlag(arg string) {
	if len(arg) == 0 {
		r.All = []string{}
		return
	}
	var tempValue = utils.SplitArgumentParser(arg)
	r.All = tempValue
}

func (a *Accounts) UnmarshalFlag(arg string) error {
	var tempValue = utils.SplitArgumentParser(arg)
	if len(tempValue) == 0 {
		return errors.New("invalid account list")
	}
	a.All = tempValue
	return nil
}
