package commands

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store/filestore"
	"log"
)

func init() {
	_, err := Parser.AddCommand("validate",
		"Runs validation scripts",
		"The `validate` command executes validation scripts.",
		&validateCommand{})
	if err != nil {
		log.Fatal(err)
	}
}

// validateCommand defines parameters for validate command
type validateCommand struct {
	projectBaseCommand
}

// Execute executes validate command
func (v *validateCommand) Execute([]string) (err error) {
	loader, projectID := filestore.NewSingleProjectLoader(v.ProjectDir)

	var project *models.DataTugProject
	if project, err = loader.GetProject(projectID); err != nil {
		return fmt.Errorf("failed to load project from [%v]: %w", v.ProjectDir, err)
	}
	fmt.Println("Validating loaded project...")
	if err := project.Validate(); err != nil {
		return err
	}
	fmt.Println("Project is valid.")
	return nil
}
