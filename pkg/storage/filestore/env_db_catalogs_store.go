package filestore

import (
	"context"
	"path/filepath"

	"github.com/datatug/datatug-core/pkg/datatug"
)

var _ datatug.EnvDbCatalogStore = (*fsEnvDbCatalogStore)(nil)

func newFsEnvCatalogsStore(environmentsDirPath string) fsEnvDbCatalogStore {
	s := fsEnvDbCatalogStore{
		fsProjectItemsStore: newFileProjectItemsStore[datatug.EnvDbCatalogs, *datatug.EnvDbCatalog, datatug.EnvDbCatalog](
			environmentsDirPath, dbCatalogFileSuffix),
	}
	s.dirPath = environmentsDirPath
	return s
}

type fsEnvDbCatalogStore struct {
	fsProjectItemsStore[datatug.EnvDbCatalogs, *datatug.EnvDbCatalog, datatug.EnvDbCatalog]
}

func (s fsEnvDbCatalogStore) getDirPath(envID string) string {
	return filepath.Join(s.dirPath, envID, EnvDbCatalogsFolder)
}

func (s fsEnvDbCatalogStore) LoadEnvDbCatalogs(ctx context.Context, envID string, o ...datatug.StoreOption) (datatug.EnvDbCatalogs, error) {
	dirPath := s.getDirPath(envID)
	return s.loadProjectItems(ctx, dirPath, o...)
}

func (s fsEnvDbCatalogStore) LoadEnvDbCatalog(ctx context.Context, envID, serverID, catalogID string, o ...datatug.StoreOption) (datatug.EnvDbCatalog, error) {
	//TODO implement me
	panic("implement me")
}

func (s fsEnvDbCatalogStore) SaveEnvDbCatalog(ctx context.Context, envID, serverID, catalogID string, catalogs *datatug.EnvDbCatalog) error {
	//TODO implement me
	panic("implement me")
}

func (s fsEnvDbCatalogStore) SaveEnvDbCatalogs(ctx context.Context, envID, serverID, catalogID string, catalogs datatug.EnvDbCatalogs) error {
	//TODO implement me
	panic("implement me")
}

func (s fsEnvDbCatalogStore) DeleteEnvDbCatalog(ctx context.Context, envID, serverID, catalogID string) error {
	//TODO implement me
	panic("implement me")
}

//// LoadEnvironmentCatalog return information about environment DB
//func (store fsEnvDbCatalogStore) LoadEnvironmentCatalog() (envDb *datatug.EnvDb, err error) {
//	filePath := path.Join(store.envsDirPath, store.envID, EnvDbCatalogsFolder, store.catalogID, jsonFileName(store.catalogID, dbCatalogFileSuffix))
//	envDb = new(datatug.EnvDb)
//	if err = readJSONFile(filePath, true, envDb); err != nil {
//		err = fmt.Errorf("failed to load environment DB catalog [%v] from env [%v] from project [%v]: %w", store.catalogID, store.envID, store.projectID, err)
//		return nil, err
//	}
//	envDb.ID = store.catalogID
//	if err = envDb.Validate(); err != nil {
//		return nil, fmt.Errorf("loaded environmend DB catalog file is invalid: %w", err)
//	}
//	return
//}
