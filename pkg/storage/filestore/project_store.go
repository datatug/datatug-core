package filestore

import (
	"context"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/datatug2md"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ datatug.ProjectStore = (*fsProjectStore)(nil)

func newFsProjectStore(projectID string, projectPath string) fsProjectStore {
	return fsProjectStore{
		projectID:                   projectID,
		projectPath:                 projectPath,
		readmeEncoder:               datatug2md.NewEncoder(),
		fsBoardsStore:               newFsBoardsStore(projectPath),
		fsQueriesStore:              newFsQueriesStore(projectPath),
		fsEntitiesStore:             newFsEntitiesStore(projectPath),
		fsFoldersStore:              newFsFoldersStore(projectPath),
		fsEnvironmentsStore:         newFsEnvironmentsStore(projectPath),
		fsEnvDbServersStore:         newFsEnvDbServersStore(projectPath),
		fsEnvDbCatalogStore:         newFsEnvCatalogsStore(projectPath),
		fsRecordsetDefinitionsStore: newFsRecordsetDefinitionsStore(projectPath),
	}
}

type fsProjectStore struct {
	projectID     string
	projectPath   string
	readmeEncoder datatug.ReadmeEncoder
	fsBoardsStore
	fsQueriesStore
	fsEntitiesStore
	fsFoldersStore
	fsEnvironmentsStore
	fsEnvDbServersStore
	fsEnvDbCatalogStore
	fsRecordsetDefinitionsStore
}

func (s fsProjectStore) LoadRecordsetData(ctx context.Context, id string) (datatug.Recordset, error) {
	return s.fsRecordsetDefinitionsStore.LoadRecordsetData(ctx, id)
}

func (s fsProjectStore) LoadRecordsetDefinitions(ctx context.Context, o ...datatug.StoreOption) ([]*datatug.RecordsetDefinition, error) {
	return s.fsRecordsetDefinitionsStore.LoadRecordsetDefinitions(ctx, o...)
}

func (s fsProjectStore) LoadRecordsetDefinition(ctx context.Context, id string, o ...datatug.StoreOption) (*datatug.RecordsetDefinition, error) {
	return s.fsRecordsetDefinitionsStore.LoadRecordsetDefinition(ctx, id, o...)
}

func (s fsProjectStore) LoadEnvironmentSummary(ctx context.Context, id string) (*datatug.EnvironmentSummary, error) {
	return s.fsEnvironmentsStore.LoadEnvironmentSummary(ctx, id)
}

func (s fsProjectStore) LoadProjDbServerSummary(ctx context.Context, id string) (*datatug.ProjDbServerSummary, error) {
	dirPath := path.Join(s.projectPath, storage.ServersFolder)
	server, err := s.fsEnvDbServersStore.loadProjectItem(ctx, dirPath, id, "")
	if err != nil {
		return nil, err
	}
	summary := &datatug.ProjDbServerSummary{
		DbServer: server.ServerReference,
	}
	return summary, nil
}

func (s fsProjectStore) SaveProjDbServer(ctx context.Context, server *datatug.ProjDbServer) error {
	dirPath := path.Join(s.projectPath, storage.ServersFolder)
	envDbServer := &datatug.EnvDbServer{
		ServerReference: server.Server,
	}
	return s.fsEnvDbServersStore.saveProjectItem(ctx, dirPath, envDbServer)
}

func (s fsProjectStore) DeleteProjDbServer(ctx context.Context, id string) error {
	dirPath := path.Join(s.projectPath, storage.ServersFolder)
	return s.fsEnvDbServersStore.deleteProjectItem(ctx, dirPath, id)
}

func (s fsProjectStore) ProjectID() string {
	return s.projectID
}

func (s fsProjectStore) LoadEnvironments(ctx context.Context, o ...datatug.StoreOption) (environments datatug.Environments, err error) {
	return s.fsEnvironmentsStore.LoadEnvironments(ctx, o...)
}

func (s fsProjectStore) LoadProjDbServers(ctx context.Context, o ...datatug.StoreOption) (datatug.ProjDbServers, error) {
	dirPath := path.Join(s.projectPath, storage.ServersFolder)
	items, err := s.fsEnvDbServersStore.loadProjectItems(ctx, dirPath, o...)
	if err != nil {
		return nil, err
	}
	servers := make(datatug.ProjDbServers, len(items))
	for i, item := range items {
		servers[i] = &datatug.ProjDbServer{
			Server: item.ServerReference,
		}
	}
	return servers, nil
}

type fsProjectStoreRef struct {
	fsProjectStore
}

//func (ps fsProjectStoreRef) Project() storage.ProjectStore {
//	return ps.fsProjectStore
//}

//func (store fsProjectStore) ProjectID() string {
//	return store.projectID
//}

//func (store fsProjectStore) DbModels() storage.DbModelsStore {
//	return newFsDbModelsStore(store)
//}

//func (store fsProjectStore) Environments() storage.environmentsStore {
//	return newFsEnvironmentsStore(store)
//}

//func (store fsProjectStore) Entities() storage.entitiesStore {
//	panic("implement me")
//}

//func (store fsProjectStore) DbServers() storage.DbServersStore {
//	return newFsDbServersStore(store)
//}

//func (store fsProjectStore) Recordsets() storage.RecordsetsStore {
//	panic("implement me")
//}

//func (store fsProjectStore) Folders() storage.FoldersStore {
//	return fsFoldersStore{fsProjectStore: store}
//}

//func (store fsProjectStore) Queries() storage.QueriesStore {
//	return newFsQueriesStore(store)
//}
