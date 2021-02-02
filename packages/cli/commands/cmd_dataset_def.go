package commands

import (
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
	// TODO: Implement "dataset def" command
	dataset, err := v.loader.LoadDatasetDefinition(v.projectID, v.Dataset)
	if err != nil {
		return err
	}
	dataset.ID = v.Dataset
	encoder := yaml.NewEncoder(os.Stdout)
	return encoder.Encode(dataset)
}
