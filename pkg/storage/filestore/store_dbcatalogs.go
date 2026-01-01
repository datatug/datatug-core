package filestore

import (
	"path"

	"github.com/datatug/datatug-core/pkg/storage"
)

var _ storage.DbCatalogsStore = (*fsDbCatalogsStore)(nil)

type fsDbCatalogsStore struct {
	catalogsDirPath string
	fsDbServerStore
}

func newFsDbCatalogsStore(fsDbServerStore fsDbServerStore) fsDbCatalogsStore {
	return fsDbCatalogsStore{
		catalogsDirPath: path.Join(fsDbServerStore.projectPath, storage.ServersFolder, storage.DbFolder, fsDbServerStore.dbServer.Driver, fsDbServerStore.dbServer.Host, storage.EnvDbCatalogsFolder),
		fsDbServerStore: fsDbServerStore,
	}
}

func (store fsDbCatalogsStore) Server() storage.DbServerStore {
	return store.fsDbServerStore
}

func (store fsDbCatalogsStore) DbCatalog(id string) storage.DbCatalogStore {
	return store.catalog(id)
}

func (store fsDbCatalogsStore) catalog(id string) fsDbCatalogStore {
	return newFsDbCatalogStore(id, store)
}
