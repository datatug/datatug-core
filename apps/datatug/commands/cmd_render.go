package commands

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/cli"
	"github.com/datatug/datatug/packages/storage"
	"github.com/datatug/datatug/packages/storage/filestore"
	"log"
	"os"
)

func renderCommandArgs(p cli.Parser) {
	_, err := p.AddCommand("render",
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

	store, _ := filestore.NewSingleProjectStore(storage.SingleProjectID, v.ProjectDir)
	projectStore := store.GetProjectStore(storage.SingleProjectID)
	datatugProject, err := projectStore.LoadProject(context.Background())
	if err != nil {
		return fmt.Errorf("failed to load project by ID=%v: %w", v.projectID, err)
	}

	log.Println("Saving project", datatugProject.ID, "...")
	if err = projectStore.SaveProject(context.Background(), *datatugProject); err != nil {
		err = fmt.Errorf("failed to save datatug project: %w", err)
		return err
	}

	return err
}
