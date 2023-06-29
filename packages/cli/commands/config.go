package commands

import (
	"fmt"
	"github.com/mitchellh/go-homedir"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path"
)

// ConfigFile hold DataTug executable configuration for commands like `serve`
type ConfigFile struct {
	Path     string                   `yaml:"-"` // TODO: Document intended use
	Projects map[string]ProjectConfig `yaml:"projects,omitempty"`
	Client   *ClientConfig            `yaml:"client,omitempty"`
	Server   *ServerConfig            `yaml:"server,omitempty"`
}

type UrlConfig struct {
	Host string `yaml:"host,omitempty"`
	Port int    `yaml:"port,omitempty"`
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

const (
	DefaultHost       = "localhost"
	DefaultClientPort = 4200
	DefaultServerPort = 8989
)

func getConfig() (config ConfigFile, err error) {
	var f *os.File
	var homeDir string
	if homeDir, err = homedir.Dir(); err != nil {
		err = fmt.Errorf("Failed to get user's home dir: %w", err)
		return
	}

	config.Path = ".datatug.yaml"
	if homeDir != "" {
		config.Path = path.Join(homeDir, config.Path)
	}
	if f, err = os.Open(config.Path); err != nil {
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("failed to closed config file opened for read: %v", err)
		}
	}()
	decoder := yaml.NewDecoder(f)
	if err = decoder.Decode(&config); err != nil {
		return
	}
	return
}

func getServerConfig(config ConfigFile) ServerConfig {
	if config.Server == nil {
		config.Server = &ServerConfig{
			UrlConfig: UrlConfig{
				Host: DefaultHost,
				Port: DefaultServerPort,
			},
		}
	}
	if config.Server.Host == "" {
		config.Server.Host = DefaultHost
	}
	if config.Server.Port == 0 {
		config.Server.Port = DefaultServerPort
	}
	return *config.Server
}

type ConfigFormat string

const (
	ConfigFormatYaml ConfigFormat = "yaml"
)

func printConfig(config ConfigFile, format ConfigFormat, w io.Writer) (err error) {
	var encoder interface {
		Encode(v interface{}) error
	}
	switch format {
	case "yaml":
		encoder = yaml.NewEncoder(w)
	default:
		return fmt.Errorf("unsupported format: %v", format)
	}
	return encoder.Encode(config)
}

func printConfigSection(section interface{}, format ConfigFormat, w io.Writer) (err error) {
	var encoder interface {
		Encode(v interface{}) error
	}
	switch format {
	case "yaml":
		encoder = yaml.NewEncoder(w)
	default:
		return fmt.Errorf("unsupported format: %v", format)
	}
	return encoder.Encode(section)
}
