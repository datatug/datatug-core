package filestore

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/models2md"
	"os"
	"path"
)

func (s FileSystemSaver) writeTableReadme(dirPath string, table *models.Table) func() error {
	return func() error {
		file, _ := os.OpenFile(path.Join(dirPath, "README.md"), os.O_CREATE, os.ModePerm)
		defer func() {
			_ = file.Close()
		}()
		if err := models2md.EncodeTable(file, table); err != nil {
			return err
		}
		return nil
	}
}

func (s FileSystemSaver) writeProjectReadme(project models.DataTugProject) error {
	file, _ := os.OpenFile(path.Join(s.path, DatatugFolder, "README.md"), os.O_CREATE, os.ModePerm)
	defer func() {
		_ = file.Close()
	}()
	return models2md.EncodeProjectSummary(file, project)
}

