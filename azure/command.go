package azure

import "github.com/jessevdk/go-flags"

type Options struct {
	TenantID            string `long:"tenant-id" default:"68f381e3-46da-47b9-ba57-6f322b8f0da1" description:"Azure Tenant ID"`
	RootManagementGroup string `long:"root-group" short:"g" default:"tamu" description:"management group IDs to begin search for subscriptions"`
	// ExcludeManagementGroups []string `long:"exclude-groups" short:"e" default:"sandbox" description:"comma-separated list of one or more nested management group IDs to exclude"`
}

func AddCommand(p *flags.Parser) {
	options = &Options{}
	p.AddCommand("azure", "Initialize Azure", "Initialize Azure", options)
}
