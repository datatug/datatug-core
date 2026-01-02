package filestore

import (
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
		fsDbModelsStore:             newFsDbModelsStore(projectPath),
		fsQueriesStore:              newFsQueriesStore(projectPath),
		fsEntitiesStore:             newFsEntitiesStore(projectPath),
		fsFoldersStore:              newFsFoldersStore(projectPath),
		fsEnvironmentsStore:         newFsEnvironmentsStore(projectPath),
		fsEnvDbServersStore:         newFsEnvDbServersStore(projectPath),
		fsEnvDbCatalogStore:         newFsEnvCatalogsStore(projectPath),
		fsProjDbDriversStore:        newFsProjDbDriversStore(projectPath),
		fsRecordsetDefinitionsStore: newFsRecordsetDefinitionsStore(projectPath),
	}
}

type fsProjectStore struct {
	projectID     string
	projectPath   string
	readmeEncoder datatug.ReadmeEncoder
	fsBoardsStore
	fsDbModelsStore
	fsQueriesStore
	fsEntitiesStore
	fsFoldersStore
	fsEnvironmentsStore
	fsEnvDbServersStore
	fsEnvDbCatalogStore
	fsProjDbDriversStore
	fsRecordsetDefinitionsStore
}

func (s fsProjectStore) ProjectID() string {
	return s.projectID
}

//type fsProjectStoreRef struct {
//	fsProjectStore
//}

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
//	return newFsProjDbServersStore(store)
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
