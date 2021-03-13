package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"io"
	"log"
	"os"
	"path"
)

func (s fileSystemSaver) saveDbServers(dbServers models.ProjDbServers, project models.DatatugProject) (err error) {
	if len(dbServers) == 0 {
		log.Println("Project have no DB servers to save.")
		return nil
	}
	log.Printf("Saving %v DB servers...\n", len(project.DbServers))
	err = parallel.Run(
		func() (err error) {
			return s.saveDbServersJSON(dbServers)
		},
		func() (err error) {
			return s.saveDbServersReadme(dbServers)
		},
		func() (err error) {
			return s.saveItems("servers", len(dbServers), func(i int) func() error {
				return func() error {
					return s.SaveDbServer(*dbServers[i], project)
				}
			})
		},
	)
	if err != nil {
		return fmt.Errorf("failed to save DB servers: %w", err)
	}
	log.Printf("Saved %v DB servers.", len(project.DbServers))
	return nil
}

func (s fileSystemSaver) saveDbServersJSON(dbServers models.ProjDbServers) error {
	servers := make(models.ServerReferences, len(dbServers))
	for i, server := range dbServers {
		servers[i] = server.Server
	}
	dirPath := path.Join(s.projDirPath, ServersFolder, DbFolder)
	if err := s.saveJSONFile(dirPath, "servers.json", servers); err != nil {
		return fmt.Errorf("failed to save list of servers as JSON file: %w", err)
	}
	return nil
}

func (s fileSystemSaver) saveDbServersReadme(dbServers models.ProjDbServers) error {
	return nil
}

// SaveDbServer saves ServerReference
func (s fileSystemSaver) SaveDbServer(dbServer models.ProjDbServer, project models.DatatugProject) (err error) {
	if err = dbServer.Validate(); err != nil {
		return fmt.Errorf("db server is not valid: %w", err)
	}
	dbServerDirPath := path.Join(s.projDirPath, DatatugFolder, ServersFolder, DbFolder, dbServer.Server.Driver, dbServer.Server.FileName())
	if err := os.MkdirAll(dbServerDirPath, 0777); err != nil {
		return fmt.Errorf("failed to created server directory: %w", err)
	}
	err = parallel.Run(
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
	if err != nil {
		return fmt.Errorf("failed to save DB server [%v]: %w", dbServer.ID, err)
	}
	return nil
}

func (s fileSystemSaver) saveDbServerReadme(dbServer models.ProjDbServer, dbServerDirPath string, project models.DatatugProject) error {
	return saveReadme(dbServerDirPath, "DB server", func(w io.Writer) error {
		if err := s.readmeEncoder.DbServerToReadme(w, project.Repository, dbServer); err != nil {
			return fmt.Errorf("failed to write README.md for DB server: %w", err)
		}
		return nil
	})
}

func (s fileSystemSaver) saveDbServerJSON(dbServer models.ProjDbServer, dbServerDirPath string, _ models.DatatugProject) error {
	log.Println("s.projDirPath:", s.projDirPath)
	log.Println("dbServerDirPath:", dbServerDirPath)
	if err := os.MkdirAll(dbServerDirPath, 0777); err != nil {
		return fmt.Errorf("failed to create a directory for DB server files: %w", err)
	}
	serverFile := models.ProjDbServerFile{
		ServerReference: dbServer.Server,
	}
	if len(dbServer.Catalogs) > 0 {
		serverFile.Catalogs = make([]string, len(dbServer.Catalogs))
		for i, catalog := range dbServer.Catalogs {
			serverFile.Catalogs[i] = catalog.ID
		}
	}
	if err := s.saveJSONFile(dbServerDirPath, jsonFileName(dbServer.Server.FileName(), dbServerFileSuffix), serverFile); err != nil {
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
