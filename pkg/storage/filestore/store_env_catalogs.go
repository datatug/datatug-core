package filestore

import (
	"github.com/datatug/datatug-core/pkg/storage"
	"path"
)

var _ storage.EnvDbCatalogsStore = (*fsEnvCatalogsStore)(nil)

type fsEnvCatalogsStore struct {
	fsEnvServerStore
	envCatalogsDirPath string
}

func newFsEnvCatalogsStore(fsEnvServersStore fsEnvServerStore) fsEnvCatalogsStore {
	return fsEnvCatalogsStore{
		fsEnvServerStore:   fsEnvServersStore,
		envCatalogsDirPath: path.Join(fsEnvServersStore.envServersPath),
	}
}

func (store fsEnvCatalogsStore) Catalog(id string) storage.EnvDbCatalogStore {
	return store.catalog(id)
}

func (store fsEnvCatalogsStore) catalog(id string) fsEnvCatalogStore {
	return newFsEnvCatalogStore(id, store)
}
