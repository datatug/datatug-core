package filestore

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"github.com/datatug/datatug/packages/storage"
	"io"
	"log"
	"os"
	"path"
)

var _ storage.DbServersStore = (*fsDbServersStore)(nil)

type fsDbServersStore struct {
	fsProjectStoreRef
}

func newFsDbServersStore(fsProjectStore fsProjectStore) fsDbServersStore {
	return fsDbServersStore{
		fsProjectStoreRef: fsProjectStoreRef{fsProjectStore},
	}
}

func (store fsDbServersStore) DbServer(id models.ServerReference) storage.DbServerStore {
	return store.dbServer(id)
}

func (store fsDbServersStore) dbServer(dbServer models.ServerReference) storage.DbServerStore {
	return newFsDbServerStore(dbServer, store)
}

func (store fsDbServersStore) saveDbServers(ctx context.Context, dbServers models.ProjDbServers, project models.DatatugProject) (err error) {
	if len(dbServers) == 0 {
		log.Println("GetProjectStore have no DB servers to save.")
		return nil
	}
	log.Printf("Saving %v DB servers...\n", len(project.DbServers))
	dbServersDirPath := path.Join(store.projectPath, DatatugFolder, ServersFolder, DbFolder)
	err = parallel.Run(
		func() (err error) {
			return store.saveDbServersJSON(dbServersDirPath, dbServers)
		},
		func() (err error) {
			return store.saveDbServersReadme(dbServers)
		},
		func() (err error) {
			return saveItems("servers", len(dbServers), func(i int) func() error {
				return func() error {
					dbServer := *dbServers[i]
					dbServerStore := newFsDbServerStore(dbServer.Server, store)
					return dbServerStore.SaveDbServer(ctx, *dbServers[i], project)
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

func (store fsDbServersStore) saveDbServersJSON(dbServersDirPath string, dbServers models.ProjDbServers) error {
	servers := make(models.ServerReferences, len(dbServers))
	for i, server := range dbServers {
		servers[i] = server.Server
	}
	if err := saveJSONFile(dbServersDirPath, "servers.json", servers); err != nil {
		return fmt.Errorf("failed to save list of servers as JSON file: %w", err)
	}
	return nil
}

func (store fsDbServersStore) saveDbServersReadme(dbServers models.ProjDbServers) error {
	panic(fmt.Sprintf("not implemented saving of dbServers=%v", dbServers))
}

// SaveDbServer saves ServerReference
func (store fsDbServerStore) SaveDbServer(_ context.Context, dbServer models.ProjDbServer, project models.DatatugProject) (err error) {
	if err = dbServer.Validate(); err != nil {
		return fmt.Errorf("db server is not valid: %w", err)
	}
	dbServerDirPath := path.Join(store.projectPath, DatatugFolder, ServersFolder, DbFolder, dbServer.Server.Driver, dbServer.Server.FileName())
	if err := os.MkdirAll(dbServerDirPath, 0777); err != nil {
		return fmt.Errorf("failed to created server directory: %w", err)
	}
	err = parallel.Run(
		func() error {
			return store.saveDbServerJSON(dbServer, dbServerDirPath, project)
		},
		func() error {
			return store.saveDbServerReadme(dbServer, dbServerDirPath, project)
		},
		func() error {
			catalogStore := newFsDbCatalogsStore(store).catalog(dbServer.ID)
			if err = catalogStore.saveDbCatalogs(dbServer, project.Repository); err != nil {
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

func (store fsDbServerStore) saveDbServerReadme(dbServer models.ProjDbServer, dbServerDirPath string, project models.DatatugProject) error {
	return saveReadme(dbServerDirPath, "DB server", func(w io.Writer) error {
		if err := store.readmeEncoder.DbServerToReadme(w, project.Repository, dbServer); err != nil {
			return fmt.Errorf("failed to write README.md for DB server: %w", err)
		}
		return nil
	})
}

func (store fsDbServerStore) saveDbServerJSON(dbServer models.ProjDbServer, dbServerDirPath string, _ models.DatatugProject) error {
	log.Println("store.projDirPath:", store.projectPath)
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
	if err := saveJSONFile(dbServerDirPath, jsonFileName(dbServer.Server.FileName(), dbServerFileSuffix), serverFile); err != nil {
		return fmt.Errorf("failed to save DB server JSON file: %w", err)
	}
	return nil
}
