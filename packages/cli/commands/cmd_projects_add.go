package commands

import (
	"errors"
	"fmt"
	"github.com/datatug/datatug/packages/cli/config"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
)

type addProjectCommand struct {
	projectBaseCommand
}

// Execute executes "projects add" command
func (v *addProjectCommand) Execute(_ []string) error {
	_, _ = fmt.Println("Reading settings file...")
	settings, err := config.GetSettings()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to read settings file: %v", err)
	}
	projectID := strings.ToLower(v.ProjectName)
	project, ok := settings.Projects[projectID]
	if ok { // Project with requested name already added to settings
		if project.Path == v.ProjectDir { // Attempt to add the same project with same path
			return nil // No problem, just do nothing.
		}
		return fmt.Errorf("project with name [%v] already added to settings with path: %v", projectID, project.Path)
	}
	settings.Projects[projectID] = config.ProjectConfig{Path: v.ProjectDir}
	if err = saveConfig(settings); err != nil {
		return fmt.Errorf("failed to save settings: %w", err)
	}
	return nil
}

func saveConfig(config config.Settings) error {
	f, err := os.Create(config.Path)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("failed to close config file opened for writing: %v", err)
		}
	}()
	if config.Server != nil && config.Server.IsEmpty() {
		config.Server = nil
	}
	if config.Client != nil && config.Client.IsEmpty() {
		config.Client = nil
	}
	encoder := yaml.NewEncoder(f)
	if err = encoder.Encode(config); err != nil {
		return err
	}
	return nil
}
