package commands

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datatug/datatug/apps/datatug/uimodels"
	"github.com/datatug/datatug/apps/firestoreviewer"
	"github.com/datatug/datatug/packages/auth"
	"github.com/datatug/datatug/packages/auth/gcloud"
	cliv3 "github.com/urfave/cli/v3"
	"os"
)

func DatatugCommand() *cliv3.Command {
	return &cliv3.Command{
		Action: datatugCommandAction,
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

func datatugCommandAction(_ context.Context, _ *cliv3.Command) error {

	datatugApp := uimodels.DatatugMainMenu()
	p := tea.NewProgram(datatugApp, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		// Ensure the error is printed to console explicitly
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return nil
}
