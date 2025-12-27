package filestore

import (
	"context"
	"sync"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/datatug2md"
)

func newFsProjectStore(projectID string, projectPath string) fsProjectStore {
	return fsProjectStore{
		projectID:     projectID,
		projectPath:   projectPath,
		readmeEncoder: datatug2md.NewEncoder(),
		fsBoardsStore: fsBoardsStore{
			fsProjectItemsStore: newFsProjectItemsStore[datatug.Boards, *datatug.Board, datatug.Board](
				projectPath, BoardsFolder, boardFileSuffix,
			),
		},
		fsQueriesStore: fsQueriesStore{
			fsProjectItemsStore: newFsProjectItemsStore[datatug.QueryDefs, *datatug.QueryDef, datatug.QueryDef](
				projectPath, QueriesFolder, querySQLFileSuffix,
			),
		},
	}
}

var _ datatug.ProjectStore = (*fsProjectStore)(nil)

// fsProjectStore

type fsProjectStore struct {
	projectID     string
	projectPath   string
	projFileMutex *sync.Mutex
	readmeEncoder datatug.ReadmeEncoder
	fsBoardsStore
	fsQueriesStore
}

func (s fsProjectStore) LoadRecordsetData(ctx context.Context, id string) (datatug.Recordset, error) {
	//TODO implement me
	panic("implement me")
}

func (s fsProjectStore) LoadRecordsetDefinitions(ctx context.Context, o ...datatug.StoreOption) ([]*datatug.RecordsetDefinition, error) {
	//TODO implement me
	panic("implement me")
}

func (s fsProjectStore) LoadRecordsetDefinition(ctx context.Context, id string) (*datatug.RecordsetDefinition, error) {
	//TODO implement me
	panic("implement me")
}

func (s fsProjectStore) LoadFolders(ctx context.Context, o ...datatug.StoreOption) (*datatug.Folder, error) {
	//TODO implement me
	panic("implement me")
}

func (s fsProjectStore) SaveFolder(ctx context.Context, path string, folder *datatug.Folder) error {
	//TODO implement me
	panic("implement me")
}

func (s fsProjectStore) DeleteFolder(ctx context.Context, id string) error {
	//TODO implement me
	panic("implement me")
}

func (s fsProjectStore) LoadQuery(ctx context.Context, id string) (*datatug.QueryDefWithFolderPath, error) {
	return s.GetQuery(ctx, id)
}

func (s fsProjectStore) SaveQuery(ctx context.Context, query *datatug.QueryDefWithFolderPath) error {
	return s.fsQueriesStore.saveProjectItem(ctx, &query.QueryDef)
}

func (s fsProjectStore) DeleteQuery(ctx context.Context, id string) error {
	return s.fsQueriesStore.deleteProjectItem(ctx, id)
}

func (s fsProjectStore) LoadEntities(ctx context.Context, o ...datatug.StoreOption) (datatug.Entities, error) {
	return s.loadEntities(ctx, o...)
}

func (s fsProjectStore) LoadEntity(ctx context.Context, id string, o ...datatug.StoreOption) (*datatug.Entity, error) {
	return s.loadEntity(ctx, id, o...)
}

func (s fsProjectStore) SaveEntity(ctx context.Context, entity *datatug.Entity) error {
	return s.saveEntity(ctx, entity)
}

func (s fsProjectStore) DeleteEntity(ctx context.Context, id string) error {
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
	return s.loadEnvironments(ctx, o...) // ./environments_store.go
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
