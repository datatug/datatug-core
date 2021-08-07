package commands

import (
	"fmt"
	"log"
)

func init() {
	datasetCmd, err := Parser.AddCommand("dataset",
		"Recordset commands: def, data",
		"Recordset commands: def, data",
		&datasetCommand{})
	if err != nil {
		log.Fatal(err)
	}
	datasetCmd.Aliases = []string{"ds"}
	datasetCmd.SubcommandsOptional = true
	if _, err = datasetCmd.AddCommand("def",
		"Shows dataset definition",
		"Shows dataset definition",
		&datasetDefCommand{}); err != nil {
		log.Fatal(fmt.Errorf("failed to add 'dataset def' command"))
	}

	if _, err = datasetCmd.AddCommand("data",
		"Shows dataset data",
		"Shows dataset data",
		&datasetDataCommand{}); err != nil {
		log.Fatal(fmt.Errorf("failed to add 'dataset data' command"))
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
