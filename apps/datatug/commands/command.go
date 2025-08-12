package commands

import (
	"github.com/datatug/datatug/apps/firestoreviewer"
	"github.com/datatug/datatug/packages/auth"
	"github.com/datatug/datatug/packages/auth/gcloud"
	cliv3 "github.com/urfave/cli/v3"
)

func DatatugCommand() *cliv3.Command {
	return &cliv3.Command{
		Commands: []*cliv3.Command{
			initCommand(),
			auth.AuthCommand(),
			gcloud.GoogleCloudCommand(),
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
