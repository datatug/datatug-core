package filestore

import (
	"sync"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/datatug2md"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ storage.ProjectStore = (*fsProjectStore)(nil)

type fsProjectStore struct {
	projectID     string
	projectPath   string
	projFileMutex *sync.Mutex
	readmeEncoder datatug.ReadmeEncoder
}

type fsProjectStoreRef struct {
	fsProjectStore
}

func (ps fsProjectStoreRef) Project() storage.ProjectStore {
	return ps.fsProjectStore
}

func (store fsProjectStore) ProjectID() string {
	return store.projectID
}

func (store fsProjectStore) DbModels() storage.DbModelsStore {
	return newFsDbModelsStore(store)
}

func (store fsProjectStore) Environments() storage.EnvironmentsStore {
	return newFsEnvironmentsStore(store)
}

func (store fsProjectStore) Entities() storage.EntitiesStore {
	panic("implement me")
}

func (store fsProjectStore) DbServers() storage.DbServersStore {
	return newFsDbServersStore(store)
}

func (store fsProjectStore) Recordsets() storage.RecordsetsStore {
	panic("implement me")
}

func (store fsProjectStore) Folders() storage.FoldersStore {
	return fsFoldersStore{fsProjectStore: store}
}

func (store fsProjectStore) Boards() storage.BoardsStore {
	return newFsBoardsStore(store)
}
func (store fsProjectStore) Queries() storage.QueriesStore {
	return newFsQueriesStore(store)
}

func newFsProjectStore(id string, projectPath string) fsProjectStore {
	return fsProjectStore{
		projectID:     id,
		projectPath:   projectPath,
		readmeEncoder: datatug2md.NewEncoder(),
	}
}
