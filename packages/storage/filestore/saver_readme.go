package filestore

import (
	"github.com/datatug/datatug/packages/models"
	"os"
	"path"
)

func (s fsProjectStore) writeProjectReadme(project models.DatatugProject) error {
	filePath := path.Join(s.projectPath, DatatugFolder, "README.md")
	file, _ := os.Create(filePath)
	defer func() {
		_ = file.Close()
	}()
	return s.readmeEncoder.ProjectSummaryToReadme(file, project)
}
