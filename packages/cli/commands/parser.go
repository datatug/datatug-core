package commands

import "github.com/jessevdk/go-flags"

//var options Options

// Parser parses command line arguments
var Parser interface {
	Parse() ([]string, error)
	AddCommand(command string, shortDescription string, longDescription string, data interface{}) (*flags.Command, error)
} = flags.NewParser(nil, flags.Default)
