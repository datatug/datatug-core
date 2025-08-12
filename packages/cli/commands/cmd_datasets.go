package commands

import (
	"context"
	"fmt"
	"log"
)

func datasetsCommandArgs(p Parser) {
	_, err := p.AddCommand("datasets",
		"Lists datasets if no sub-command provided",
		"Lists datasets if no sub-command provided",
		&datasetsCommand{})
	if err != nil {
		log.Fatal(err)
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
