package main

import (
	"os"

	"github.com/jessevdk/go-flags"
	"github.com/tamu-edu/aiphelper/aws"
	"github.com/tamu-edu/aiphelper/azure"
)

// https://lightstep.com/blog/getting-real-with-command-line-arguments-and-goflags/
// var (
// 	arguments = new(config.Parameters)
// )

func main() {

	p := flags.NewParser(nil, flags.Default)

	aws.AddCommand(p)
	azure.AddCommand(p)

	_, err := p.Parse()
	if err != nil {
		os.Exit(-1)
	}

	switch p.Active.Name {
	case "aws":
		aws.Init()
	case "azure":
		azure.Init()
	}
}
