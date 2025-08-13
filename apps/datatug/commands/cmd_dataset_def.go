package commands

import (
	"context"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
	"os"
)

type datasetDefCommand struct {
	datasetBaseCommand
}

func datasetDefCommandAction(_ context.Context, _ *cli.Command) error {
	v := &datasetDefCommand{}
	if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}
	ctx := context.Background()
	// TODO: Implement "dataset def" consoleCommand
	dataset, err := v.store.GetProjectStore(v.projectID).Recordsets().Recordset(v.Dataset).LoadRecordsetDefinition(ctx)
	if err != nil {
		return err
	}
	dataset.ID = v.Dataset
	encoder := yaml.NewEncoder(os.Stdout)
	return encoder.Encode(dataset)
}

func datasetDefCommandArgs() *cli.Command {
	return &cli.Command{
		Name:        "dataset-def",
		Usage:       "Outputs dataset definition in YAML",
		Description: "Displays dataset (recordset) definition in YAML",
		Action:      datasetDefCommandAction,
	}
}
