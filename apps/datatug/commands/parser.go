package commands

import (
	"github.com/datatug/datatug/apps/firestoreviewer"
	cliv3 "github.com/urfave/cli/v3"
)

func GetCommand() *cliv3.Command {
	return &cliv3.Command{
		Commands: []*cliv3.Command{
			initCommand(),
			configCommandArgs(),
			datasetCommandArgs(),
			datasetsCommandArgs(),
			demoCommandArgs(),
			updateUrlConfigCommandArgs(),
			projectsCommandArgs(),
			queriesCommandArgs(),
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
