package commands

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"log"
)

func init() {
	_, err := Parser.AddCommand("test",
		"Runs validation scripts",
		"The `test` command executes validation scripts.",
		&validateCommand{})
	if err != nil {
		log.Fatal(err)
	}
}

// validateCommand defines parameters for test command
type validateCommand struct {
	projectBaseCommand
}

// Execute executes test command
func (v *validateCommand) Execute([]string) (err error) {
	if err = v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}

	var project *models.DatatugProject
	if project, err = v.store.Project(v.projectID).LoadProject(context.Background()); err != nil {
		return fmt.Errorf("failed to load project from [%v]: %w", v.ProjectDir, err)
	}
	fmt.Println("Validating loaded project...")
	if err := project.Validate(); err != nil {
		return err
	}
	fmt.Println("Project is valid.")
	return nil
}
