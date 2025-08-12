package commands

import (
	"context"
	"fmt"
	cliv3 "github.com/urfave/cli/v3"
)

func datasetsCommandArgs() *cliv3.Command {
	return &cliv3.Command{
		Name:        "datasets",
		Usage:       "Lists datasets if no sub-command provided",
		Description: "Lists datasets if no sub-command provided",
		Action: func(ctx context.Context, c *cliv3.Command) error {
			v := &datasetsCommand{}
			return v.Execute(nil)
		},
	}
}

// datasetsCommand defines parameters for test command
type datasetsCommand struct {
	projectBaseCommand
}

// Execute executes test command
func (v *datasetsCommand) Execute([]string) error {
	if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}
	ctx := context.Background()
	datasets, err := v.store.GetProjectStore(v.projectID).Recordsets().LoadRecordsetDefinitions(ctx)
	if err != nil {
		return fmt.Errorf("failed to load datasets from [%v]: %w", v.ProjectDir, err)
	}
	for _, dataset := range datasets {
		_, _ = fmt.Println(dataset.ID)
	}
	return nil
}
