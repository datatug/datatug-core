package commands

import (
	"context"
	cliv3 "github.com/urfave/cli/v3"
)

func datasetCommandAction(_ context.Context, _ *cliv3.Command) error {
	v := &datasetCommand{}
	if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}
	// TODO: Implement "datasets show" consoleCommand
	return nil
}

func datasetCommands() *cliv3.Command {
	return &cliv3.Command{
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
