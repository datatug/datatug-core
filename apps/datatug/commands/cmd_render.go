package commands

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/storage"
	"github.com/datatug/datatug/packages/storage/filestore"
	cliv3 "github.com/urfave/cli/v3"
	"log"
	"os"
)

func renderCommandAction(_ context.Context, _ *cliv3.Command) error {
	v := &renderCommand{}
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

func renderCommandArgs() *cliv3.Command {
	return &cliv3.Command{
		Name:        "render",
		Usage:       "Renders readme.md files",
		Description: "Updates readme.md files - this is useful for updating them without scan",
		Action:      renderCommandAction,
	}
}

// scanDbCommand defines parameters for scan consoleCommand
type renderCommand struct {
	projectBaseCommand
}
