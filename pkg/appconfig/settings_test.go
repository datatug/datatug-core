package appconfig

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

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
			name: "success",
			openFile: func(name string) (io.ReadCloser, error) {
				settings := Settings{
					Projects: []*ProjectConfig{
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
				Projects: []*ProjectConfig{
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
				openFile = osOpen
			}()
			openFile = tt.openFile
			gotSettings, err := GetSettings()
			if err == nil {
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
			} else if !errors.Is(err, tt.wantErr) {
				t.Errorf("GetSettings() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
