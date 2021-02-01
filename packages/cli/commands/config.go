package commands

import (
	"fmt"
	"github.com/mitchellh/go-homedir"
	"gopkg.in/yaml.v2"
	"io"
	"os"
	"path"
)

// ConfigFile hold DataTug executable configuration for commands like `serve`
type ConfigFile struct {
	Projects []ProjectConfig `yaml:"projects"`
}

// ProjectConfig hold project configuration, specifically path to project directory
type ProjectConfig struct {
	Title string `yaml:"title,omitempty"`
	Path  string `yaml:"path"`
}

func getConfig() (config ConfigFile, err error) {
	var f *os.File
	var homeDir string
	if homeDir, err = homedir.Dir(); err != nil {
		_ = fmt.Sprintf("Failed to get user's home dir: %v", err)
		err = nil
	}

	filePath := ".datatug.yaml"
	if homeDir != "" {
		filePath = path.Join(homeDir, filePath)
	}
	if f, err = os.Open(filePath); err != nil {
		return
	}
	decoder := yaml.NewDecoder(f)
	if err = decoder.Decode(&config); err != nil {
		return
	}
	return
}

func printConfig(config ConfigFile, w io.Writer) (err error) {
	encoder := yaml.NewEncoder(w)
	if err = encoder.Encode(config); err != nil {
		return
	}
	return
}
