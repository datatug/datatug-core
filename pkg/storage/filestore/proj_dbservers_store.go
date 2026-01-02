package filestore

import (
	"context"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

func newFsProjDbServersStore(dbsPath, dbDriver string) fsProjDbServersStore {
	return fsProjDbServersStore{
		driverID: dbDriver,
		fsProjectItemsStore: newFileProjectItemsStore[datatug.ProjDbServers, *datatug.ProjDbServer, datatug.ProjDbServer](
			path.Join(dbsPath, dbDriver), storage.DbServerFileSuffix,
		),
	}
}

var _ datatug.ProjDbServersStore = (*fsProjDbServersStore)(nil)

type fsProjDbServersStore struct {
	driverID string
	fsProjectItemsStore[datatug.ProjDbServers, *datatug.ProjDbServer, datatug.ProjDbServer]
}

func (s fsProjDbServersStore) DriverID() string {
	return s.driverID
}

func (s fsProjDbServersStore) CatalogsStore(serverRef datatug.ServerRef) datatug.DbCatalogsStore {
	return newFsDbCatalogsStore(s.dirPath, serverRef)
}

func (s fsProjDbServersStore) LoadProjDbServers(ctx context.Context, o ...datatug.StoreOption) (datatug.ProjDbServers, error) {
	return s.loadProjectItems(ctx, s.dirPath, o...)
}

func (s fsProjDbServersStore) LoadProjDbServer(ctx context.Context, serverID string, o ...datatug.StoreOption) (*datatug.ProjDbServer, error) {
	return s.loadProjectItem(ctx, s.dirPath, serverID, "", o...)
}

func (s fsProjDbServersStore) SaveProjDbServer(ctx context.Context, server *datatug.ProjDbServer, o ...datatug.StoreOption) error {
	return s.saveProjectItem(ctx, s.dirPath, server, o...)
}

func (s fsProjDbServersStore) DeleteProjDbServer(ctx context.Context, serverID string) error {
	return s.deleteProjectItem(ctx, s.dirPath, serverID)
}

//func (s fsProjDbServersStore) saveDbServersJSON(dbServersDirPath string, dbServers datatug.ProjDbServers) error {
//	servers := make(datatug.ServerReferences, len(dbServers))
//	for i, server := range dbServers {
//		servers[i] = server.Server
//	}
//	if err := saveJSONFile(dbServersDirPath, "servers.json", servers); err != nil {
//		return fmt.Errorf("failed to save list of servers as JSON file: %w", err)
//	}
//	return nil
//}

//func (s fsProjDbServersStore) saveDbServersReadme(_ datatug.ProjDbServers) error {
//	return nil
//}

//func (store fsDbServerStore) saveDbServerReadme(dbServer datatug.ProjDbServer, dbServerDirPath string, project datatug.Project) error {
//	return saveReadme(dbServerDirPath, func(w io.Writer) error {
//		if err := store.readmeEncoder.DbServerToReadme(w, project.Repository, dbServer); err != nil {
//			return fmt.Errorf("failed to write README.md for DB server: %w", err)
//		}
//		return nil
//	})
//}

//func (s fsDbServerStore) saveDbServerJSON(dbServer datatug.ProjDbServer, dbServerDirPath string, _ datatug.Project) error {
//	//log.Println("s.projDirPath:", s.projectPath)
//	//log.Println("dbServerDirPath:", dbServerDirPath)
//	if err := os.MkdirAll(dbServerDirPath, 0777); err != nil {
//		return fmt.Errorf("failed to create a directory for DB server files: %w", err)
//	}
//	serverFile := datatug.ProjDbServerFile{
//		ServerRef: dbServer.Server,
//	}
//	if len(dbServer.Catalogs) > 0 {
//		serverFile.Catalogs = make([]string, len(dbServer.Catalogs))
//		for i, catalog := range dbServer.Catalogs {
//			serverFile.Catalogs[i] = catalog.ID
//		}
//	}
//	if err := saveJSONFile(dbServerDirPath, storage.JsonFileName(dbServer.Server.FileName(), storage.DbServerFileSuffix), serverFile); err != nil {
//		return fmt.Errorf("failed to save DB server JSON file: %w", err)
//	}
//	return nil
//}
