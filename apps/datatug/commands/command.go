package commands

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datatug/datatug/apps"
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
		Flags:  []cliv3.Flag{apps.TUIFlag},
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

func datatugCommandAction(_ context.Context, cmd *cliv3.Command) error {
	if !apps.TUIFlag.IsSet() {
		// Show default help text when TUI is not requested
		_ = cliv3.ShowRootCommandHelp(cmd)
		return nil
	}
	datatugApp := uimodels.DatatugAppModel()
	p := tea.NewProgram(datatugApp, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		// Ensure the error is printed to the console explicitly
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return nil
}
