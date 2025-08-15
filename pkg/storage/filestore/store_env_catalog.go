package filestore

import (
	"fmt"
	"github.com/datatug/datatug-core/pkg/models"
	"github.com/datatug/datatug-core/pkg/storage"
	"path"
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

func (store fsEnvCatalogStore) SaveDbCatalog(envServer *models.EnvDbServer) error {
	panic("not implemented?")
}

// GetEnvironmentDb return information about environment DB
func (store fsEnvCatalogStore) LoadEnvironmentCatalog() (envDb *models.EnvDb, err error) {
	filePath := path.Join(store.envsDirPath, store.envID, DbCatalogsFolder, store.catalogID, jsonFileName(store.catalogID, dbCatalogFileSuffix))
	envDb = new(models.EnvDb)
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
