package commands

import (
	"fmt"
	"github.com/datatug/datatug/packages/store"
	"github.com/datatug/datatug/packages/store/filestore"
	"log"
	"os"
)

func init() {
	_, err := Parser.AddCommand("render",
		"Renders readme.md files",
		"Updates readme.md files - this is useful for updating them without scan",
		&renderCommand{})
	if err != nil {
		log.Fatal(err)
	}
}

// scanDbCommand defines parameters for scan command
type renderCommand struct {
	projectBaseCommand
}

// Execute executes scan command
func (v *renderCommand) Execute(_ []string) error {
	if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}
	if v.projectID != "" {
		_, _ = fmt.Printf("Rendering project: %v...", v.projectID)
	} else {
		_, _ = fmt.Printf("Rendering project: %v...", v.ProjectDir)
	}
	log.Println("Initiating project...")
	if _, err := os.Stat(v.ProjectDir); os.IsNotExist(err) {
		return fmt.Errorf("ProjectDir=[%v] not found: %w", v.ProjectDir, err)
	}

	loader, projectID := filestore.NewSingleProjectLoader(v.ProjectDir)
	dataTugProject, err := loader.LoadProject(projectID)
	if err != nil {
		return fmt.Errorf("failed to load project by ID=%v: %w", v.projectID, err)
	}

	log.Println("Saving project", dataTugProject.ID, "...")
	store.Current, _ = filestore.NewSingleProjectStore(v.ProjectDir, v.projectID)
	var dal store.Interface
	if dal, err = store.NewDatatugStore(""); err != nil {
		return err
	}
	if err = dal.Save(*dataTugProject); err != nil {
		err = fmt.Errorf("failed to save datatug project [%v]: %w", v.projectID, err)
		return err
	}

	return err
}
