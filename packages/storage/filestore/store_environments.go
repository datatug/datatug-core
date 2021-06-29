package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"path"
)

var _ storage.EnvironmentStore = (*fsEnvironmentStore)(nil)

type fsEnvironmentStore struct {
	fsProjectStore
	envsDirPath string
}

func (store fsEnvironmentStore) Loader() storage.EnvironmentsLoader {
	panic("implement me")
}

func (store fsEnvironmentStore) Saver() storage.EnvironmentsSaver {
	panic("implement me")
}

func newFsEnvironmentStore(fsProjectStore fsProjectStore) fsEnvironmentStore {
	return fsEnvironmentStore{
		fsProjectStore: fsProjectStore,
		envsDirPath:    path.Join(fsProjectStore.projectPath, DatatugFolder, EnvironmentsFolder)}
}

// GetEnvironmentSummary loads environment summary
func (store fsEnvironmentStore) LoadEnvironmentSummary(envID string) (envSummary models.EnvironmentSummary, err error) {
	if envSummary, err = loadEnvFile(store.envsDirPath, envID); err != nil {
		err = fmt.Errorf("failed to load environment [%v] from project [%v]: %w", envID, store.projectID, err)
		return
	}
	return
}

// GetEnvironmentDbSummary return DB summary for specific environment
func (store fsEnvironmentStore) LoadEnvironmentDbSummary(projectID, environmentID, databaseID string) (models.DbCatalogSummary, error) {
	panic(fmt.Sprintf("implement me: %v, %v, %v", projectID, environmentID, databaseID))
}

// GetEnvironmentDb return information about environment DB
func (store fsEnvironmentStore) LoadEnvironmentCatalog(environmentID, databaseID string) (envDb *models.EnvDb, err error) {
	filePath := path.Join(store.envsDirPath, environmentID, DbCatalogsFolder, databaseID, jsonFileName(databaseID, dbCatalogFileSuffix))
	envDb = new(models.EnvDb)
	if err = readJSONFile(filePath, true, envDb); err != nil {
		err = fmt.Errorf("failed to load environment DB catalog [%v] from env [%v] from project [%v]: %w", databaseID, environmentID, store.projectID, err)
		return nil, err
	}
	envDb.ID = databaseID
	if err = envDb.Validate(); err != nil {
		return nil, fmt.Errorf("loaded environmend DB catalog file is invalid: %w", err)
	}
	return
}
