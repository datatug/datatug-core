package commands

import "log"

func init() {
	datasetCmd, err := Parser.AddCommand("dataset",
		"Dataset commands: def, data",
		"Dataset commands: def, data",
		&datasetCommand{})
	if err != nil {
		log.Fatal(err)
	}
	datasetCmd.Aliases = []string{"ds"}
	datasetCmd.SubcommandsOptional = true
	_, err = datasetCmd.AddCommand("def",
		"Shows dataset definition",
		"Shows dataset definition",
		&datasetDefCommand{})
	_, err = datasetCmd.AddCommand("data",
		"Shows dataset data",
		"Shows dataset data",
		&datasetDataCommand{})
	if err != nil {
		log.Fatal(err)
	}
}

type datasetBaseCommand struct {
	projectBaseCommand
}

// datasetCommand defines parameters for validate command
type datasetCommand struct {
	datasetBaseCommand
}

// Execute executes validate command
func (v *datasetCommand) Execute([]string) error {
	if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}
	// TODO: Implement "datasets show" command
	return nil
}
