package dtconfig

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
	Projects []*ProjectRef `yaml:"projects,omitempty" json:"projects,omitempty"`

	Client *ClientConfig `yaml:"client,omitempty" json:"client,omitempty"`
	Server *ServerConfig `yaml:"server,omitempty" json:"server,omitempty"`

	Credentials map[string][]AuthCredential `yaml:"credentials,omitempty" json:"credentials,omitempty"`
}

func (v Settings) GetProjectConfig(projectID string) *ProjectRef {
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

// ProjectRef hold project configuration, specifically path to project directory
type ProjectRef struct {
	ID    string `yaml:"id"`
	Path  string `yaml:"path,omitempty"` // Local path
	Url   string `yaml:"url,omitempty"`
	Title string `yaml:"title,omitempty"`
}

func (v ProjectRef) Validate() error {
	var empty ProjectRef
	if v == empty {
		return fmt.Errorf("is empty")
	}
	if v.ID == "" && v.Path == "" && v.Url == "" {
		return fmt.Errorf("at least one of key fields must be set: id, path, url")
	}
	return nil
}

const ConfigFileName = ".datatug.yaml"

var homedirDir = homedir.Dir

func GetConfigFilePath() string {
	configFilePath, err := homedirDir()
	if err != nil {
		panic(fmt.Errorf("failed to get user's home dir: %w", err))
	}
	return path.Join(configFilePath, ConfigFileName)
}

var standardOsOpen = func(name string) (io.ReadCloser, error) {
	return os.Open(name)
}

var osOpen = standardOsOpen

var osCreate = func(name string) (interface{ io.WriteCloser }, error) {
	return os.Create(name)
}

var getSettings = GetSettings

func GetSettings() (settings Settings, err error) {
	configFilePath := GetConfigFilePath()
	var f io.ReadCloser
	if f, err = osOpen(configFilePath); err != nil {
		return
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			fmt.Printf("failed to closed settings file opened for read: %v", closeErr)
		}
	}()
	decoder := yaml.NewDecoder(f)
	if err = decoder.Decode(&settings); err != nil {
		return
	}
	//setDefault(&settings)
	return
}

var saveSettings = SaveSettings

func SaveSettings(settings Settings) error {
	configFilePath := GetConfigFilePath()
	f, err := osCreate(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to create settings file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	if settings.Server != nil && settings.Server.IsEmpty() {
		settings.Server = nil
	}
	if settings.Client != nil && settings.Client.IsEmpty() {
		settings.Client = nil
	}

	encoder := yaml.NewEncoder(f)
	if err = encoder.Encode(settings); err != nil {
		return fmt.Errorf("failed to encode settings: %w", err)
	}
	return nil
}

func AddProjectToSettings(project ProjectRef) error {

	settings, err := getSettings()

	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to get DataTug CLI settings: %w", err)
	}

	// Check if already exists
	for _, p := range settings.Projects {
		if p.ID == project.ID {
			return fmt.Errorf("project already exists, id: %s", p.ID)
		}
	}

	settings.Projects = append(settings.Projects, &project)

	return saveSettings(settings)
}
