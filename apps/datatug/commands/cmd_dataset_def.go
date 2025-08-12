package commands

import (
	"context"
	"gopkg.in/yaml.v3"
	"os"
)

type datasetDefCommand struct {
	datasetBaseCommand
}

// Execute command
func (v *datasetDefCommand) Execute([]string) error {
	if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}
	ctx := context.Background()
	// TODO: Implement "dataset def" command
	dataset, err := v.store.GetProjectStore(v.projectID).Recordsets().Recordset(v.Dataset).LoadRecordsetDefinition(ctx)
	if err != nil {
		return err
	}
	dataset.ID = v.Dataset
	encoder := yaml.NewEncoder(os.Stdout)
	return encoder.Encode(dataset)
}
