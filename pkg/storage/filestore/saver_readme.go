package filestore

import (
	"os"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func (store fsProjectStore) writeProjectReadme(project datatug.Project) error {
	filePath := path.Join(store.projectPath, DatatugFolder, "README.md")
	file, _ := os.Create(filePath)
	defer func() {
		_ = file.Close()
	}()
	return store.readmeEncoder.ProjectSummaryToReadme(file, project)
}
