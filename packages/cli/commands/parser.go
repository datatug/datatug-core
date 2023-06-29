package commands

import "github.com/jessevdk/go-flags"

type parser interface {
	Parse() ([]string, error)
	AddCommand(command string, shortDescription string, longDescription string, data interface{}) (*flags.Command, error)
}

// Parser parses command line arguments
var Parser parser = flags.NewParser(nil, flags.Default)
