package filestore

import (
	"github.com/datatug/datatug/packages/models"
	"os"
	"path"
)

func (store fsProjectStore) writeProjectReadme(project models.DatatugProject) error {
	filePath := path.Join(store.projectPath, DatatugFolder, "README.md")
	file, _ := os.Create(filePath)
	defer func() {
		_ = file.Close()
	}()
	return store.readmeEncoder.ProjectSummaryToReadme(file, project)
}
