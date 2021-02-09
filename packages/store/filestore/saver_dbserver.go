package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"log"
	"os"
	"path"
)

// SaveDbServer saves DbServer
func (s fileSystemSaver) SaveDbServer(dbServer *models.ProjDbServer) (err error) {
	return parallel.Run(
		func() error {
			dbServerDirPath := path.Join(s.projDirPath, "servers", "db", dbServer.DbServer.Driver, dbServer.DbServer.FileName())
			log.Println("s.projDirPath:", s.projDirPath)
			log.Println("dbServerDirPath:", dbServerDirPath)
			if err := os.MkdirAll(dbServerDirPath, os.ModeDir); err != nil {
				return err
			}
			return s.saveJSONFile(dbServerDirPath, fmt.Sprintf("%v.%v.json", dbServer.DbServer.Driver, dbServer.DbServer.FileName()), models.ProjDbServerFile{})
		},
		func() error {
			if err = s.saveDatabases(dbServer.DbServer, dbServer.Databases); err != nil {
				return fmt.Errorf("failed to save environment databases: %w", err)
			}
			return nil
		},
	)
}

// DeleteDbServer deletes DB server
func (s fileSystemSaver) DeleteDbServer(dbServer models.DbServer) (err error) {
	dbServerDirPath := path.Join(s.projDirPath, "servers", "db", dbServer.Driver, dbServer.FileName())
	log.Println("Deleting folder:", dbServerDirPath)
	if err = os.RemoveAll(dbServerDirPath); err != nil {
		return fmt.Errorf("failed to remove db server directory: %w", err)
	}
	return
}
