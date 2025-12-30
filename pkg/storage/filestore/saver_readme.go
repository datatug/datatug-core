package filestore

import (
	"os"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func (s fsProjectStore) writeProjectReadme(project datatug.Project) error {
	filePath := path.Join(s.projectPath, "README.md")
	file, _ := os.Create(filePath)
	defer func() {
		_ = file.Close()
	}()
	return s.readmeEncoder.ProjectSummaryToReadme(file, project)
}
