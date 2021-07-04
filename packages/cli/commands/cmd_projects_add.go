package commands

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
)

type addProjectCommand struct {
	projectBaseCommand
}

// Execute executes "projects add" command
func (v *addProjectCommand) Execute(_ []string) error {
	_, _ = fmt.Println("Reading config file...")
	config, err := getConfig()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to read config file: %v", err)
	}
	projectID := strings.ToLower(v.ProjectName)
	project, ok := config.Projects[projectID]
	if ok { // Project with requested name already added to config
		if project.Path == v.ProjectDir { // Attempt to add the same project with same path
			return nil // No problem, just do nothing.
		}
		return fmt.Errorf("project with name [%v] already added to config with path: %v", projectID, project.Path)
	}
	config.Projects[projectID] = ProjectConfig{Path: v.ProjectDir}
	if err = saveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	return nil
}

func saveConfig(config ConfigFile) error {
	f, err := os.Create(config.Path)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("failed to close config file opened for writing: %v", err)
		}
	}()
	encoder := yaml.NewEncoder(f)
	if err = encoder.Encode(config); err != nil {
		return err
	}
	return nil
}
