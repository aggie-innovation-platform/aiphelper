package main

import (
	"os"

	"github.com/jessevdk/go-flags"
	"github.com/tamu-edu/aiphelper/aws"
	"github.com/tamu-edu/aiphelper/azure"
	"github.com/tamu-edu/aiphelper/config"
)

var (
	arguments = new(config.Parameters)
)

func main() {

	p := flags.NewParser(arguments, flags.Default)

	_, err := p.Parse()
	if err != nil {
		os.Exit(-1)
	}

	switch p.Active.Name {
	case "aws":
		aws.Init(arguments)
	case "azure":
		azure.Init(arguments)
	}
}
