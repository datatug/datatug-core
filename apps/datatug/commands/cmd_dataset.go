package commands

import (
	"context"
	cliv3 "github.com/urfave/cli/v3"
)

func datasetCommandArgs() *cliv3.Command {
	return &cliv3.Command{
		Name:        "dataset",
		Usage:       "Recordset commands: def, data",
		Description: "Recordset commands: def, data",
		Aliases:     []string{"ds"},
		Action: func(ctx context.Context, c *cliv3.Command) error {
			v := &datasetCommand{}
			return v.Execute(nil)
		},
	}
}

type datasetBaseCommand struct {
	projectBaseCommand
	Dataset string `long:"dataset"`
}

// datasetCommand defines parameters for test command
type datasetCommand struct {
	datasetBaseCommand
}

// Execute executes test command
func (v *datasetCommand) Execute([]string) error {
	if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}
	// TODO: Implement "datasets show" command
	return nil
}
