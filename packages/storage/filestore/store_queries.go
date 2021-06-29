package filestore

import (
	"github.com/datatug/datatug/packages/storage"
	"path"
)

type fsQueriesStore struct {
	fsProjectStore
	queriesDirPath string
}

func newFsQueriesStore(fsProjectStore fsProjectStore) fsQueriesStore {
	return fsQueriesStore{fsProjectStore: fsProjectStore, queriesDirPath: path.Join(fsProjectStore.projectPath, DatatugFolder, QueriesFolder)}
}

func (store fsQueriesStore) Loader() storage.QueriesLoader {
	return newFileSystemQueriesLoader(store)
}

func (store fsQueriesStore) Saver() storage.QuerySaver {
	panic("implement me")
}

var _ storage.QueriesStore = (*fsQueriesStore)(nil)
