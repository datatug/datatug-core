package commands

import (
	"fmt"
	"log"
)

func init() {
	_, err := Parser.AddCommand("datasets",
		"Lists datasets if no sub-command provided",
		"Lists datasets if no sub-command provided",
		&datasetsCommand{})
	if err != nil {
		log.Fatal(err)
	}
}

// datasetsCommand defines parameters for validate command
type datasetsCommand struct {
	projectBaseCommand
}

// Execute executes validate command
func (v *datasetsCommand) Execute([]string) error {
	if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}

	datasets, err := v.store.Project(v.projectID).Recordsets().LoadRecordsetDefinitions()
	if err != nil {
		return fmt.Errorf("failed to load datasets from [%v]: %w", v.ProjectDir, err)
	}
	for _, dataset := range datasets {
		_, _ = fmt.Println(dataset.ID)
	}
	return nil
}
