package filestore

import (
	"github.com/datatug/datatug/packages/models"
	"log"
	"os"
	"path"
)

func (s fileSystemSaver) writeTableReadme(dirPath string, catalog string, table *models.Table, dbServer models.ProjDbServer) func() error {
	return func() error {
		log.Printf("Saving readme.md for table %v.%v.%v...\n", catalog, table.Schema, table.Name)
		file, _ := os.Create(path.Join(dirPath, "README.md"))
		defer func() {
			_ = file.Close()
		}()
		if err := s.readmeEncoder.EncodeTable(file, catalog, table, dbServer); err != nil {
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
