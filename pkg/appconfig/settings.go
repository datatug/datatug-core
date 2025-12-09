package appconfig

import (
	"fmt"
	"os"
	"path"

	"github.com/mitchellh/go-homedir"
	"gopkg.in/yaml.v3"
)

// Settings hold DataTug executable configuration for commands like `serve`
type Settings struct {
	Projects []*ProjectConfig `yaml:"projects,omitempty"` // Intentionally do not use map

	Client *ClientConfig `yaml:"client,omitempty"`
	Server *ServerConfig `yaml:"server,omitempty"`

	Credentials map[string][]AuthCredential
}

func (v Settings) GetProjectConfig(projectID string) *ProjectConfig {
	for _, p := range v.Projects {
		if p.ID == projectID {
			return p
		}
	}
	return nil
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

type StoreType string

const FileStoreUrlPrefix = "file:"

// ProjectConfig hold project configuration, specifically path to project directory
type ProjectConfig struct {
	ID    string `yaml:"id"`
	Url   string `yaml:"url"`
	Title string `yaml:"title,omitempty"`
}

func (v ProjectConfig) Validate() error {
	return nil
}

const ConfigFileName = ".datatug.yaml"

func GetConfigFilePath() string {
	configFilePath, err := homedir.Dir()
	if err != nil {
		panic(fmt.Errorf("failed to get user's home dir: %w", err))
	}
	return path.Join(configFilePath, ConfigFileName)
}

func GetSettings() (settings Settings, err error) {
	configFilePath := GetConfigFilePath()
	var f *os.File
	if f, err = os.Open(configFilePath); err != nil {
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
