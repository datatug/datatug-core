package dtconfig

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/mitchellh/go-homedir"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

type errorCloser struct {
	io.Reader
}

func (ec errorCloser) Close() error {
	return errors.New("close error")
}

func TestGetSettings(t *testing.T) {
	testErr := errors.New("test error")
	tests := []struct {
		name         string
		wantSettings Settings
		openFile     func(name string) (io.ReadCloser, error)
		wantErr      error
	}{
		{
			name: "returns error if os.Open fails",
			openFile: func(name string) (io.ReadCloser, error) {
				return nil, testErr
			},
			wantErr: testErr,
		},
		{
			name: "returns error if yaml decoding fails",
			openFile: func(name string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader([]byte("invalid yaml"))), nil
			},
			wantErr: yaml.Unmarshal([]byte("invalid yaml"), &Settings{}), // just to get a yaml error
		},
		{
			name: "success with close error",
			openFile: func(name string) (io.ReadCloser, error) {
				return errorCloser{bytes.NewReader([]byte("{}"))}, nil
			},
			wantSettings: Settings{},
		},
		{
			name: "success",
			openFile: func(name string) (io.ReadCloser, error) {
				settings := Settings{
					Projects: []*ProjectRef{
						{
							ID:   "project1",
							Path: "~/datatug/project1",
						},
					},
				}
				s, err := yaml.Marshal(settings)
				return io.NopCloser(bytes.NewReader(s)), err
			},
			wantSettings: Settings{
				Projects: []*ProjectRef{
					{
						ID:   "project1",
						Path: "~/datatug/project1",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				osOpen = standardOsOpen
			}()
			osOpen = tt.openFile
			gotSettings, err := GetSettings()
			if err != nil {
				if tt.wantErr == nil {
					t.Errorf("GetSettings() unexpected error = %v", err)
				}
				// We don't check exact error for YAML decoder because it might be different, but we check presence
				return
			}
			if tt.wantErr != nil {
				t.Errorf("GetSettings() expected error = %v, got nil", tt.wantErr)
			}
			if !reflect.DeepEqual(gotSettings, tt.wantSettings) {
				if !reflect.DeepEqual(gotSettings.Projects, tt.wantSettings.Projects) {
					t.Errorf("got projects = %v, want %v", gotSettings.Projects, tt.wantSettings.Projects)
				}
				if !reflect.DeepEqual(gotSettings.Client, tt.wantSettings.Client) {
					t.Errorf("got client = %v, want %v", gotSettings.Client, tt.wantSettings.Client)
				}
				if !reflect.DeepEqual(gotSettings.Server, tt.wantSettings.Server) {
					t.Errorf("got server = %v, want %v", gotSettings.Server, tt.wantSettings.Server)
				}
				if !reflect.DeepEqual(gotSettings.Credentials, tt.wantSettings.Credentials) {
					t.Errorf("got credentials = %v, want %v", gotSettings.Credentials, tt.wantSettings.Credentials)
				}
			}
		})
	}
}

func TestGetConfigFilePath(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		path := GetConfigFilePath()
		if path == "" {
			t.Error("GetConfigFilePath() returned empty string")
		}
	})
	t.Run("panic", func(t *testing.T) {
		defer func() {
			homedirDir = homedir.Dir
		}()
		homedirDir = func() (string, error) {
			return "", errors.New("test error")
		}
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("GetConfigFilePath() should have panicked")
			}
		}()
		GetConfigFilePath()
	})
}

func TestOsOpen(t *testing.T) {
	f, err := osOpen("non-existent-file")
	if err == nil {
		_ = f.Close()
		t.Error("expected error for non-existent file")
	}
}

func TestAuthCredential_Validate(t *testing.T) {
	v := AuthCredential{}
	if err := v.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestSettings_GetProjectConfig(t *testing.T) {
	settings := Settings{
		Projects: []*ProjectRef{
			{ID: "p1"},
			{ID: "p2"},
		},
	}
	t.Run("found", func(t *testing.T) {
		got := settings.GetProjectConfig("p1")
		if got == nil || got.ID != "p1" {
			t.Errorf("GetProjectConfig() = %v, want p1", got)
		}
	})
	t.Run("not_found", func(t *testing.T) {
		got := settings.GetProjectConfig("p3")
		if got != nil {
			t.Errorf("GetProjectConfig() = %v, want nil", got)
		}
	})
}

func TestUrlConfig_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		v    *UrlConfig
		want bool
	}{
		{"nil", nil, true},
		{"empty", &UrlConfig{}, true},
		{"host", &UrlConfig{Host: "h"}, false},
		{"port", &UrlConfig{Port: 80}, false},
		{"both", &UrlConfig{Host: "h", Port: 80}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProjectConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		p       ProjectRef
		wantErr string
	}{
		{
			name:    "empty",
			p:       ProjectRef{},
			wantErr: "empty",
		},
		{
			name:    "title_only",
			p:       ProjectRef{Title: "Project 1"},
			wantErr: "at least one of key fields must be set",
		},
		{
			name: "id_only",
			p:    ProjectRef{ID: "project1"},
		},
		{
			name: "full_success",
			p: ProjectRef{
				ID:    "project1",
				Path:  "~/datatug/github.com/datatug/project1",
				Url:   "http://github.com/datatug/project1",
				Title: "Project 1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ProjectConfig.Validate() returned unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("ProjectConfig.Validate() expected error %s, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ProjectConfig.Validate() expected error to contain %v, got %v", tt.wantErr, err)
				return
			}
		})
	}
}

func TestAddProjectToSettings(t *testing.T) {
	var savedSettings *Settings

	tests := []struct {
		name         string
		project      ProjectRef
		wantErr      string
		getSettings  func() (settings Settings, err error)
		saveSettings func(settings Settings) error
	}{
		{
			name:    "add_to_empty",
			project: ProjectRef{ID: "project1", Path: "~/datatug/project1"},
			getSettings: func() (settings Settings, err error) {
				return
			},
			saveSettings: func(settings Settings) error {
				savedSettings = &settings
				return nil
			},
		},
		{
			name:    "add_2nd_project",
			project: ProjectRef{ID: "project2", Path: "~/datatug/project2"},
			getSettings: func() (settings Settings, err error) {
				return Settings{
					Projects: []*ProjectRef{
						{ID: "project1", Path: "~/datatug/project1"},
					},
				}, nil
			},
			saveSettings: func(settings Settings) error {
				savedSettings = &settings
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.getSettings != nil {
				getSettings = tt.getSettings
				defer func() {
					getSettings = GetSettings
				}()
			}
			if tt.saveSettings != nil {
				saveSettings = tt.saveSettings
				defer func() {
					saveSettings = SaveSettings
				}()
			}
			savedSettings = nil
			err := AddProjectToSettings(tt.project)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("AddProjectToSettings() unexpected error = %v", err)
				}
				assert.NotNil(t, savedSettings)
				projIndex := len(savedSettings.Projects) - 1
				assert.Equal(t, &tt.project, savedSettings.Projects[projIndex])
				if len(savedSettings.Projects) > 1 {
					for i, p := range savedSettings.Projects {
						if i != projIndex {
							assert.NotEqual(t, tt.project.ID, p.ID)
						}
					}
				}
				return
			}
			if err == nil {
				t.Errorf("AddProjectToSettings() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("AddProjectToSettings() expected error = %v, got %v", tt.wantErr, err)
			}
		})
	}
}
