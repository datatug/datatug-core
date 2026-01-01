package filestore

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestLoadRootDatatugFile(t *testing.T) {
	type args struct {
		dir string
	}
	tests := []struct {
		name             string
		args             args
		fileContent      string
		wantRepoRootFile *datatug.RepoRootFile
		wantErr          assert.ErrorAssertionFunc
	}{
		{
			name: "2_projects",
			args: args{
				dir: "~/datatug/project1",
			},
			fileContent: `# 2 projects file
projects:
  - datatug/project1
  - datatug/project2`,
			wantRepoRootFile: &datatug.RepoRootFile{
				Projects: []string{
					"datatug/project1",
					"datatug/project2",
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldOsOpen := osOpen
			defer func() { osOpen = oldOsOpen }()
			osOpen = func(name string) (f io.ReadCloser, err error) {
				f = io.NopCloser(strings.NewReader(tt.fileContent))
				return
			}
			gotRepoRootFile, err := LoadRootDatatugFile(tt.args.dir)
			if !tt.wantErr(t, err, fmt.Sprintf("LoadRootDatatugFile(%v)", tt.args.dir)) {
				return
			}
			assert.Equalf(t, tt.wantRepoRootFile, gotRepoRootFile, "LoadRootDatatugFile(%v)", tt.args.dir)
		})
	}
}
