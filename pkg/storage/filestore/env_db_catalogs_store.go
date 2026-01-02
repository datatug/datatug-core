package filestore

import (
	"context"
	"path/filepath"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ datatug.EnvDbCatalogStore = (*fsEnvDbCatalogStore)(nil)

func newFsEnvCatalogsStore(environmentsDirPath string) fsEnvDbCatalogStore {
	s := fsEnvDbCatalogStore{
		fsProjectItemsStore: newFileProjectItemsStore[datatug.DbCatalogs, *datatug.DbCatalog, datatug.DbCatalog](
			environmentsDirPath, storage.DbCatalogFileSuffix),
	}
	s.dirPath = environmentsDirPath
	return s
}

type fsEnvDbCatalogStore struct {
	fsProjectItemsStore[datatug.DbCatalogs, *datatug.DbCatalog, datatug.DbCatalog]
}

func (s fsEnvDbCatalogStore) getDirPath(envID string) string {
	return filepath.Join(s.dirPath, storage.EnvironmentsFolder, envID, storage.EnvDbCatalogsFolder)
}

func (s fsEnvDbCatalogStore) LoadEnvDbCatalogs(ctx context.Context, envID string, o ...datatug.StoreOption) (datatug.DbCatalogs, error) {
	dirPath := s.getDirPath(envID)
	return s.loadProjectItems(ctx, dirPath, o...)
}

func (s fsEnvDbCatalogStore) LoadEnvDbCatalog(ctx context.Context, envID, serverID, catalogID string, o ...datatug.StoreOption) (datatug.DbCatalog, error) {
	dirPath := filepath.Join(s.dirPath, storage.EnvironmentsFolder, envID, storage.ServersFolder, serverID, storage.EnvDbCatalogsFolder)
	item, err := s.loadProjectItem(ctx, dirPath, catalogID, "", o...)
	if err != nil {
		return datatug.DbCatalog{}, err
	}
	return *item, nil
}

func (s fsEnvDbCatalogStore) SaveEnvDbCatalog(ctx context.Context, envID, serverID, catalogID string, catalog *datatug.DbCatalog) error {
	_ = catalogID
	dirPath := filepath.Join(s.dirPath, storage.EnvironmentsFolder, envID, storage.ServersFolder, serverID, storage.EnvDbCatalogsFolder)
	return s.saveProjectItem(ctx, dirPath, catalog)
}

func (s fsEnvDbCatalogStore) SaveEnvDbCatalogs(ctx context.Context, envID, serverID, catalogID string, catalogs datatug.DbCatalogs) error {
	_ = catalogID
	dirPath := filepath.Join(s.dirPath, storage.EnvironmentsFolder, envID, storage.ServersFolder, serverID, storage.EnvDbCatalogsFolder)
	return s.saveProjectItems(ctx, dirPath, catalogs)
}

func (s fsEnvDbCatalogStore) DeleteEnvDbCatalog(ctx context.Context, envID, serverID, catalogID string) error {
	dirPath := filepath.Join(s.dirPath, storage.EnvironmentsFolder, envID, storage.ServersFolder, serverID, storage.EnvDbCatalogsFolder)
	return s.deleteProjectItem(ctx, dirPath, catalogID)
}

//// LoadEnvironmentCatalog return information about environment DB
//func (store fsEnvDbCatalogStore) LoadEnvironmentCatalog() (envDb *datatug.EnvDb, err error) {
//	filePath := path.Join(store.envsDirPath, store.envID, EnvDbCatalogsFolder, store.catalogID, JsonFileName(store.catalogID, DbCatalogFileSuffix))
//	envDb = new(datatug.EnvDb)
//	if err = readJSONFile(filePath, true, envDb); err != nil {
//		err = fmt.Errorf("failed to load environment DB catalog [%v] from env [%v] from project [%v]: %w", store.catalogID, store.envID, store.projectID, err)
//		return nil, err
//	}
//	envDb.ID = store.catalogID
//	if err = envDb.ValidateWithOptions(); err != nil {
//		return nil, fmt.Errorf("loaded environmend DB catalog file is invalid: %w", err)
//	}
//	return
//}
