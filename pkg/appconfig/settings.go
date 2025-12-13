package appconfig

import (
	"fmt"
	"io"
	"os"
	"path"

	"github.com/mitchellh/go-homedir"
	"gopkg.in/yaml.v3"
)

// Settings hold DataTug executable configuration for commands like `serve`
type Settings struct {
	// Intentionally do not use map
	Projects []*ProjectConfig `yaml:"projects,omitempty" json:"projects,omitempty"`

	Client *ClientConfig `yaml:"client,omitempty" json:"client,omitempty"`
	Server *ServerConfig `yaml:"server,omitempty" json:"server,omitempty"`

	Credentials map[string][]AuthCredential `yaml:"credentials,omitempty" json:"credentials,omitempty"`
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

//const FileStoreUrlPrefix = "file:"

// ProjectConfig hold project configuration, specifically path to project directory
type ProjectConfig struct {
	ID    string `yaml:"id"`
	Path  string `yaml:"path,omitempty"` // Local path
	Url   string `yaml:"url,omitempty"`
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

var osOpen = func(name string) (io.ReadCloser, error) {
	return os.Open(name)
}

var openFile = osOpen

func GetSettings() (settings Settings, err error) {
	configFilePath := GetConfigFilePath()
	var f io.ReadCloser
	if f, err = openFile(configFilePath); err != nil {
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
	//setDefault(&settings)
	return
}
