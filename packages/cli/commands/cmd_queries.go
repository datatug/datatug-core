package commands

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"log"
	"os"
)

func init() {
	_, err := Parser.AddCommand("queries",
		"Lists queries if no sub-command provided",
		"Lists queries if no sub-command provided",
		&queriesCommand{})
	if err != nil {
		log.Fatal(err)
	}
}

// datasetsCommand defines parameters for validate command
type queriesCommand struct {
	projectBaseCommand
}

// Execute executes validate command
func (v *queriesCommand) Execute([]string) error {
	if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}

	queries, err := v.loader.LoadQueries(v.projectID, "")
	if err != nil {
		return fmt.Errorf("failed to load datasets from [%v]: %w", v.ProjectDir, err)
	}
	encoder := yaml.NewEncoder(os.Stdout)
	if err = encoder.Encode(queries); err != nil {
		return err
	}
	return nil
}
