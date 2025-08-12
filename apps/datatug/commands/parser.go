package commands

import (
	"github.com/datatug/datatug/apps/firestoreviewer"
	cliv3 "github.com/urfave/cli/v3"
)

func GetCommand() *cliv3.Command {
	return &cliv3.Command{
		Commands: []*cliv3.Command{
			initCommand(),
			configCommand(),
			datasetCommands(),
			datasetDefCommandArgs(),
			datasetDataCommandArgs(),
			datasetsCommandArgs(),
			demoCommandArgs(),
			updateUrlConfigCommandArgs(),
			projectsCommandArgs(),
			projectsAddCommandArgs(),
			queriesCommand(),
			renderCommandArgs(),
			scanCommandArgs(),
			serveCommandArgs(),
			showCommandArgs(),
			uiCommandArgs(),
			testCommandArgs(),
			consoleCommandArgs(),
			firestoreviewer.FirestoreCommand(),
		},
	}
}
