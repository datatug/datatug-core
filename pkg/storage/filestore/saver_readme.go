package filestore

import (
	"io"
	"path"

	"github.com/datatug/datatug-core/internal/plainfs"
	"github.com/datatug/datatug-core/pkg/datatug"
)

func (s fsProjectStore) writeProjectReadme(project datatug.Project) error {
	return plainfs.WriteFile(s.projectPath, path.Join(s.projectPath, "README.md"), func(w io.Writer) error {
		return s.readmeEncoder.ProjectSummaryToReadme(w, project)
	})
}
