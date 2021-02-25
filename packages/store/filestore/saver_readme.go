package filestore

import (
	"github.com/datatug/datatug/packages/models"
	"os"
	"path"
)

func (s fileSystemSaver) writeTableReadme(table *models.Table, save saveDbServerObjContext) func() error {
	return func() error {
		//log.Printf("Saving readme.md for table %v.%v.%v...\n", catalog, table.Schema, table.Name)
		file, _ := os.Create(path.Join(save.dirPath, "README.md"))
		defer func() {
			_ = file.Close()
		}()
		if err := s.readmeEncoder.EncodeTable(file, save.repository, save.catalog, table, save.dbServer); err != nil {
			return err
		}
		return nil
	}
}

func (s fileSystemSaver) writeProjectReadme(project models.DataTugProject) error {
	filePath := path.Join(s.projDirPath, DatatugFolder, "README.md")
	file, _ := os.Create(filePath)
	defer func() {
		_ = file.Close()
	}()
	return s.readmeEncoder.EncodeProjectSummary(file, project)
}
