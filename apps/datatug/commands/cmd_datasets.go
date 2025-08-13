package commands

import (
	"context"
	"fmt"
	"github.com/urfave/cli/v3"
)

func datasetsCommandAction(_ context.Context, _ *cli.Command) error {
	v := &datasetsCommand{}
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

func datasetsCommandArgs() *cli.Command {
	return &cli.Command{
		Name:   "datasets",
		Usage:  "List and manage datasets for current DataTug project",
		Action: datasetsCommandAction,
	}
}

// datasetsCommand defines parameters for test consoleCommand
type datasetsCommand struct {
	projectBaseCommand
}
