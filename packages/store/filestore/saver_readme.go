package filestore

import (
	"github.com/datatug/datatug/packages/models"
	"os"
	"path"
)

func (s fileSystemSaver) writeTableReadme(dirPath string, catalog string, table *models.Table) func() error {
	return func() error {
		file, _ := os.Create(path.Join(dirPath, "README.md"))
		defer func() {
			_ = file.Close()
		}()
		if err := s.readmeEncoder.EncodeTable(file, catalog, table); err != nil {
			return err
		}
		return nil
	}
}

func (s fileSystemSaver) writeProjectReadme(project models.DataTugProject) error {
	file, _ := os.OpenFile(path.Join(s.projDirPath, DatatugFolder, "README.md"), os.O_CREATE, os.ModePerm)
	defer func() {
		_ = file.Close()
	}()
	return s.readmeEncoder.EncodeProjectSummary(file, project)
}
