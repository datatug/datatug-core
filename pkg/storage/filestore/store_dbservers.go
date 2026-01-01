package filestore

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/parallel"
	"github.com/datatug/datatug-core/pkg/storage"
)

//var _ storage.DbServersStore = (*fsDbServersStore)(nil)

type fsDbServersStore struct {
	fsProjectStoreRef
}

func newFsDbServersStore(fsProjectStore fsProjectStore) fsDbServersStore {
	return fsDbServersStore{
		fsProjectStoreRef: fsProjectStoreRef{fsProjectStore},
	}
}

func (store fsDbServersStore) DbServer(id datatug.ServerReference) storage.DbServerStore {
	return store.dbServer(id)
}

func (store fsDbServersStore) dbServer(dbServer datatug.ServerReference) storage.DbServerStore {
	return newFsDbServerStore(dbServer, store)
}

func (store fsDbServersStore) saveDbServers(ctx context.Context, dbServers datatug.ProjDbServers, project datatug.Project) (err error) {
	if len(dbServers) == 0 {
		log.Println("GetProjectStore have no DB servers to save.")
		return nil
	}
	//log.Printf("Saving %v DB servers...\n", len(project.DbServers))
	dbServersDirPath := path.Join(store.projectPath, storage.ServersFolder, storage.DbFolder)
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
	//log.Printf("Saved %v DB servers.", len(project.DbServers))
	return nil
}

func (store fsDbServersStore) saveDbServersJSON(dbServersDirPath string, dbServers datatug.ProjDbServers) error {
	servers := make(datatug.ServerReferences, len(dbServers))
	for i, server := range dbServers {
		servers[i] = server.Server
	}
	if err := saveJSONFile(dbServersDirPath, "servers.json", servers); err != nil {
		return fmt.Errorf("failed to save list of servers as JSON file: %w", err)
	}
	return nil
}

func (store fsDbServersStore) saveDbServersReadme(_ datatug.ProjDbServers) error {
	return nil
}

// SaveDbServer saves ServerReference
func (store fsDbServerStore) SaveDbServer(_ context.Context, dbServer datatug.ProjDbServer, project datatug.Project) (err error) {
	if err = dbServer.Validate(); err != nil {
		return fmt.Errorf("db server is not valid: %w", err)
	}
	dbServerDirPath := path.Join(store.projectPath, storage.ServersFolder, storage.DbFolder, dbServer.Server.Driver, dbServer.Server.FileName())
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

func (store fsDbServerStore) saveDbServerReadme(dbServer datatug.ProjDbServer, dbServerDirPath string, project datatug.Project) error {
	return saveReadme(dbServerDirPath, func(w io.Writer) error {
		if err := store.readmeEncoder.DbServerToReadme(w, project.Repository, dbServer); err != nil {
			return fmt.Errorf("failed to write README.md for DB server: %w", err)
		}
		return nil
	})
}

func (store fsDbServerStore) saveDbServerJSON(dbServer datatug.ProjDbServer, dbServerDirPath string, _ datatug.Project) error {
	//log.Println("store.projDirPath:", store.projectPath)
	//log.Println("dbServerDirPath:", dbServerDirPath)
	if err := os.MkdirAll(dbServerDirPath, 0777); err != nil {
		return fmt.Errorf("failed to create a directory for DB server files: %w", err)
	}
	serverFile := datatug.ProjDbServerFile{
		ServerReference: dbServer.Server,
	}
	if len(dbServer.Catalogs) > 0 {
		serverFile.Catalogs = make([]string, len(dbServer.Catalogs))
		for i, catalog := range dbServer.Catalogs {
			serverFile.Catalogs[i] = catalog.ID
		}
	}
	if err := saveJSONFile(dbServerDirPath, storage.JsonFileName(dbServer.Server.FileName(), storage.DbServerFileSuffix), serverFile); err != nil {
		return fmt.Errorf("failed to save DB server JSON file: %w", err)
	}
	return nil
}
