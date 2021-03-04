package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"log"
	"os"
	"path"
)

func (s fileSystemSaver) saveDbServers(dbServers models.ProjDbServers, project models.DataTugProject) (err error) {
	return parallel.Run(
		func() (err error) {
			servers := make([]models.ServerReference, len(dbServers))
			for i, server := range dbServers {
				servers[i] = server.Server
			}
			dirPath := path.Join(s.projDirPath, "servers", "db")
			if err := s.saveJSONFile(dirPath, "servers.json", servers); err != nil {
				return fmt.Errorf("failed to save list of servers as JSON file: %w", err)
			}
			return nil
		},
		func() (err error) {
			return s.saveItems("dbservers", len(dbServers), func(i int) func() error {
				return func() error {
					return s.SaveDbServer(*dbServers[i], project)
				}
			})
		},
	)
}

// SaveDbServer saves ServerReference
func (s fileSystemSaver) SaveDbServer(dbServer models.ProjDbServer, project models.DataTugProject) (err error) {
	dbServerDirPath := path.Join(s.projDirPath, DatatugFolder, "servers", "db", dbServer.Server.Driver, dbServer.Server.FileName())
	return parallel.Run(
		func() error {
			return s.saveDbServerJSON(dbServer, dbServerDirPath, project)
		},
		func() error {
			return s.saveDbServerReadme(dbServer, dbServerDirPath, project)
		},
		func() error {
			if err = s.saveDbCatalogs(dbServer, project.Repository); err != nil {
				return fmt.Errorf("failed to save DB catalogs: %w", err)
			}
			return nil
		},
	)
}

func (s fileSystemSaver) saveDbServerReadme(dbServer models.ProjDbServer, dbServerDirPath string, project models.DataTugProject) error {
	filePath := path.Join(dbServerDirPath, "README.md")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to created README.md for DB server: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()
	if err := s.readmeEncoder.DbServerToReadme(f, project.Repository, dbServer); err != nil {
		return fmt.Errorf("failed to write README.md for DB server: %w", err)
	}
	return nil
}

func (s fileSystemSaver) saveDbServerJSON(dbServer models.ProjDbServer, dbServerDirPath string, _ models.DataTugProject) error {
	log.Println("s.projDirPath:", s.projDirPath)
	log.Println("dbServerDirPath:", dbServerDirPath)
	if err := os.MkdirAll(dbServerDirPath, 0777); err != nil {
		return fmt.Errorf("failed to create a directory for DB server files: %w", err)
	}
	fileId := fmt.Sprintf("%v.%v", dbServer.Server.Driver, dbServer.Server.FileName())
	serverFile := models.ProjDbServerFile{
		ServerReference: dbServer.Server,
	}
	if len(dbServer.Catalogs) > 0 {
		serverFile.Catalogs = make([]string, len(dbServer.Catalogs))
		for i, catalog := range dbServer.Catalogs {
			serverFile.Catalogs[i] = catalog.ID
		}
	}

	if err := s.saveJSONFile(dbServerDirPath, jsonFileName(fileId, dbServerFileSuffix), serverFile); err != nil {
		return fmt.Errorf("failed to save DB server JSON file: %w", err)
	}
	return nil
}

// DeleteDbServer deletes DB server
func (s fileSystemSaver) DeleteDbServer(dbServer models.ServerReference) (err error) {
	dbServerDirPath := path.Join(s.projDirPath, "servers", "db", dbServer.Driver, dbServer.FileName())
	log.Println("Deleting folder:", dbServerDirPath)
	if err = os.RemoveAll(dbServerDirPath); err != nil {
		return fmt.Errorf("failed to remove db server directory: %w", err)
	}
	return
}
