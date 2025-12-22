package filestore

import (
	"fmt"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ storage.EnvDbCatalogStore = (*fsEnvCatalogStore)(nil)

type fsEnvCatalogStore struct {
	catalogID string
	fsEnvCatalogsStore
}

func newFsEnvCatalogStore(catalogID string, fsEnvCatalogsStore fsEnvCatalogsStore) fsEnvCatalogStore {
	return fsEnvCatalogStore{catalogID: catalogID, fsEnvCatalogsStore: fsEnvCatalogsStore}
}

func (store fsEnvCatalogStore) Catalogs() storage.EnvDbCatalogsStore {
	return store.fsEnvCatalogsStore
}

func (store fsEnvCatalogStore) SaveDbCatalog(_ *datatug.EnvDbServer) error {
	panic("not implemented?")
}

// LoadEnvironmentCatalog return information about environment DB
func (store fsEnvCatalogStore) LoadEnvironmentCatalog() (envDb *datatug.EnvDb, err error) {
	filePath := path.Join(store.envsDirPath, store.envID, DbCatalogsFolder, store.catalogID, jsonFileName(store.catalogID, dbCatalogFileSuffix))
	envDb = new(datatug.EnvDb)
	if err = readJSONFile(filePath, true, envDb); err != nil {
		err = fmt.Errorf("failed to load environment DB catalog [%v] from env [%v] from project [%v]: %w", store.catalogID, store.envID, store.projectID, err)
		return nil, err
	}
	envDb.ID = store.catalogID
	if err = envDb.Validate(); err != nil {
		return nil, fmt.Errorf("loaded environmend DB catalog file is invalid: %w", err)
	}
	return
}
