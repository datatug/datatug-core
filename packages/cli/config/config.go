package config

import (
	"fmt"
	"github.com/mitchellh/go-homedir"
	"gopkg.in/yaml.v3"
	"os"
	"path"
)

// Settings hold DataTug executable configuration for commands like `serve`
type Settings struct {
	Path     string                   `yaml:"-"` // TODO: Document intended use
	Projects map[string]ProjectConfig `yaml:"projects,omitempty"`
	Client   *ClientConfig            `yaml:"client,omitempty"`
	Server   *ServerConfig            `yaml:"server,omitempty"`
}

// UrlConfig holds host name and port
type UrlConfig struct {
	Host string `yaml:"host,omitempty"`
	Port int    `yaml:"port,omitempty"`
}

func (v *UrlConfig) IsEmpty() bool {
	return v == nil || v.Port == 0 && v.Host == ""
}

type ClientConfig struct {
	UrlConfig `yaml:",inline"`
}

type ServerConfig struct {
	UrlConfig `yaml:",inline"`
}

// ProjectConfig hold project configuration, specifically path to project directory
type ProjectConfig struct {
	Title string `yaml:"title,omitempty"`
	Path  string `yaml:"path"`
}

func GetSettings() (settings Settings, err error) {
	var f *os.File
	var homeDir string
	if homeDir, err = homedir.Dir(); err != nil {
		err = fmt.Errorf("Failed to get user's home dir: %w", err)
		return
	}

	settings.Path = ".datatug.yaml"
	if homeDir != "" {
		settings.Path = path.Join(homeDir, settings.Path)
	}
	if f, err = os.Open(settings.Path); err != nil {
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("failed to closed settings file opened for read: %v", err)
		}
	}()
	decoder := yaml.NewDecoder(f)
	if err = decoder.Decode(&settings); err != nil {
		return
	}
	setDefault(&settings)
	return
}
