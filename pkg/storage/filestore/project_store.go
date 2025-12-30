package filestore

import (
	"context"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/datatug2md"
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
	//TODO implement me
	panic("implement me")
}

func (s fsProjectStore) LoadRecordsetDefinitions(ctx context.Context, o ...datatug.StoreOption) ([]*datatug.RecordsetDefinition, error) {
	//TODO implement me
	panic("implement me")
}

func (s fsProjectStore) LoadRecordsetDefinition(ctx context.Context, id string, o ...datatug.StoreOption) (*datatug.RecordsetDefinition, error) {
	//TODO implement me
	panic("implement me")
}

func (s fsProjectStore) LoadEnvironmentSummary(ctx context.Context, id string) (*datatug.EnvironmentSummary, error) {
	//TODO implement me
	panic("implement me")
}

func (s fsProjectStore) LoadProjDbServerSummary(_ context.Context, id string) (*datatug.ProjDbServerSummary, error) {
	_ = id
	//TODO implement me
	panic("implement me")
}

func (s fsProjectStore) SaveProjDbServer(ctx context.Context, server *datatug.ProjDbServer) error {
	//TODO implement me
	panic("implement me")
}

func (s fsProjectStore) DeleteProjDbServer(ctx context.Context, id string) error {
	//TODO implement me
	panic("implement me")
}

func (s fsProjectStore) ProjectID() string {
	return s.projectID
}

func (s fsProjectStore) LoadEnvironments(ctx context.Context, o ...datatug.StoreOption) (environments datatug.Environments, err error) {
	return s.fsEnvironmentsStore.LoadEnvironments(ctx, o...)
}

func (s fsProjectStore) LoadProjDbServers(ctx context.Context, o ...datatug.StoreOption) (datatug.ProjDbServers, error) {
	//TODO implement me
	panic("implement me")
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
