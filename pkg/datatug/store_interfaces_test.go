package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectVisibility_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       ProjectVisibility
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "public",
			v:       PublicProject,
			wantErr: assert.NoError,
		},
		{
			name:    "private",
			v:       PrivateProject,
			wantErr: assert.NoError,
		},
		{
			name:    "unknown",
			v:       9,
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, tt.v.Validate(), "ValidateWithOptions()")
		})
	}
}
