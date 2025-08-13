package commands

import (
	"context"
	"github.com/urfave/cli/v3"
)

func datasetCommandAction(_ context.Context, _ *cli.Command) error {
	v := &datasetCommand{}
	if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}
	// TODO: Implement "datasets show" consoleCommand
	return nil
}

func datasetCommands() *cli.Command {
	return &cli.Command{
		Name:        "dataset",
		Usage:       "Recordset commands: def, data",
		Description: "Recordset commands: def, data",
		Aliases:     []string{"ds"},
		Action:      datasetCommandAction,
	}
}

type datasetBaseCommand struct {
	projectBaseCommand
	Dataset string `long:"dataset"`
}

// datasetCommand defines parameters for test consoleCommand
type datasetCommand struct {
	datasetBaseCommand
}
