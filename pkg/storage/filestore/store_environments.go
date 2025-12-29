package filestore

import (
	"context"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
)

var _ datatug.EnvironmentsStore = (*fsEnvironmentsStore)(nil)

func newFsEnvironmentsStore(projectPath string) fsEnvironmentsStore {
	return fsEnvironmentsStore{
		fsProjectItemsStore: newFsProjectItemsStore[datatug.Environments, *datatug.Environment, datatug.Environment](
			path.Join(projectPath, EnvironmentsFolder), environmentFileSuffix,
		),
	}
}

type fsEnvironmentsStore struct {
	fsProjectItemsStore[datatug.Environments, *datatug.Environment, datatug.Environment]
}

func (s fsEnvironmentsStore) SaveEnvironments(ctx context.Context, envs datatug.Environments) error {
	return s.saveProjectItems(ctx, s.dirPath, envs)
}

func (s fsEnvironmentsStore) LoadEnvironments(ctx context.Context, o ...datatug.StoreOption) (datatug.Environments, error) {
	return s.loadProjectItems(ctx, s.dirPath, o...)
}

func (s fsEnvironmentsStore) LoadEnvironment(ctx context.Context, id string, o ...datatug.StoreOption) (*datatug.Environment, error) {
	return s.loadProjectItem(ctx, s.dirPath, id, "", o...)
}

func (s fsEnvironmentsStore) LoadEnvironmentSummary(ctx context.Context, id string) (envSummary *datatug.EnvironmentSummary, err error) {
	return loadEnvFile(path.Join(s.dirPath, id), id)
}

func (s fsEnvironmentsStore) SaveEnvironment(ctx context.Context, env *datatug.Environment) error {
	return s.saveProjectItem(ctx, s.dirPath, env)
}

func (s fsEnvironmentsStore) DeleteEnvironment(ctx context.Context, id string) error {
	return s.deleteProjectItem(ctx, s.dirPath, id)
}

//// LoadEnvironmentDbSummary return DB summary for specific environment
//func (store fsEnvironmentStore) LoadEnvironmentDbSummary(databaseID string) (datatug.DbCatalogSummary, error) {
//	panic(fmt.Sprintf("implement me: %v, %v, %v", store.projectID, store.envID, databaseID))
//}
//
//func (store fsEnvironmentStore) saveEnvironment(_ context.Context, env datatug.Environment) (err error) {
//	dirPath := path.Join(store.projectPath, DatatugFolder, EnvironmentsFolder, env.ID)
//	log.Printf("Saving environment [%v]: %v", env.ID, dirPath)
//	if err = os.MkdirAll(dirPath, 0777); err != nil {
//		return fmt.Errorf("failed to create environemtn folder: %w", err)
//	}
//	return parallel.Run(
//		func() error {
//			if err = saveJSONFile(dirPath, jsonFileName(env.ID, environmentFileSuffix), datatug.EnvironmentFile{ID: env.ID}); err != nil {
//				return fmt.Errorf("failed to write environment json to file: %w", err)
//			}
//			return nil
//		},
//		func() error {
//			envServers := newFsEnvServersStore(store)
//			if err = envServers.saveEnvServers(env.DbServers); err != nil {
//				return fmt.Errorf("failed to save environment servers: %w", err)
//			}
//			return nil
//		},
//	)
//}
