package datatug

import (
	"testing"
)

func TestApiService_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       ApiService
		wantErr bool
	}{
		{
			name: "valid",
			v: ApiService{
				ProjectItem: ProjectItem{
					ProjItemBrief: ProjItemBrief{
						ID:    "s1",
						Title: "Service 1",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid_id",
			v: ApiService{
				ProjectItem: ProjectItem{
					ProjItemBrief: ProjItemBrief{
						ID: "",
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("ApiService.ValidateWithOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApiEndpoint_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       ApiEndpoint
		wantErr bool
	}{
		{
			name: "valid",
			v: ApiEndpoint{
				ProjectItem: ProjectItem{
					ProjItemBrief: ProjItemBrief{
						ID: "e1",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid_id",
			v: ApiEndpoint{
				ProjectItem: ProjectItem{
					ProjItemBrief: ProjItemBrief{
						ID: "",
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("ApiEndpoint.ValidateWithOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
