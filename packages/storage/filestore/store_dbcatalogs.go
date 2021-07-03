package filestore

import (
	"github.com/datatug/datatug/packages/storage"
	"path"
)

var _ storage.DbCatalogsStore = (*fsDbCatalogsStore)(nil)

type fsDbCatalogsStore struct {
	catalogsDirPath string
	fsDbServerStore
}

func newFsDbCatalogsStore(fsDbServerStore fsDbServerStore) fsDbCatalogsStore {
	return fsDbCatalogsStore{
		catalogsDirPath: path.Join(fsDbServerStore.projectPath, DatatugFolder, ServersFolder, DbFolder, fsDbServerStore.dbServer.Driver, fsDbServerStore.dbServer.Host, DbCatalogsFolder),
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
