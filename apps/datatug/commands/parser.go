package commands

import (
	"github.com/datatug/datatug/apps/firestoreviewer"
	"github.com/datatug/datatug/packages/cli"
	"github.com/jessevdk/go-flags"
)

func newParser() cli.Parser {
	return flags.NewParser(nil, flags.Default)
}

func GetParser() cli.Parser {
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
	firestoreviewer.AddFirestoreCommand(p)
	return p
}
