package commands

import (
	"github.com/jessevdk/go-flags"
)

func newParser() Parser {
	return flags.NewParser(nil, flags.Default)
}

func GetParser() Parser {
	var p = newParser()
	initCommandArgs(p)
	configCommandArgs(p)
	datasetCommandArgs(p)
	datasetsCommandArgs(p)
	demoCommandArgs(p)
	updateUrlConfigCommandArgs(p)
	projectsCommandArgs(p)
	queriesCommandArgs(p)
	renderCommandArgs(p)
	scanCommandArgs(p)
	serveCommandArgs(p)
	showCommandArgs(p)
	uiCommandArgs(p)
	testCommandArgs(p)
	consoleCommandArgs(p)
	return p
}

type Parser interface {
	Parse() ([]string, error)
	AddCommand(command string, shortDescription string, longDescription string, data interface{}) (*flags.Command, error)
}
