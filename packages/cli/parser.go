package cli

import "github.com/jessevdk/go-flags"

type Parser interface {
	Parse() ([]string, error)
	AddCommand(command string, shortDescription string, longDescription string, data interface{}) (*flags.Command, error)
}
