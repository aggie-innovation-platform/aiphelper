package azure

import "github.com/jessevdk/go-flags"

type Options struct {
	TenantID            string `long:"tenant-id" default:"68f381e3-46da-47b9-ba57-6f322b8f0da1" description:"Azure Tenant ID"`
	EnumManagementGroup bool   `long:"enum-mgmt-group" short:"g" description:"Use an Azure Management Group to enumerate descendants for a list of Subscriptions"`
	RootManagementGroup string `long:"root-group" default:"tamu" description:"management group IDs to begin search for subscriptions"`
	// ExcludeManagementGroups []string `long:"exclude-groups" short:"e" default:"sandbox" description:"comma-separated list of one or more nested management group IDs to exclude"`
}

func AddCommand(p *flags.Parser) {
	options = &Options{}
	p.AddCommand("azure", "Initialize Azure", "Initialize Azure", options)
}
