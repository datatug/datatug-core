package filestore

import (
	"context"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ datatug.DbModelsStore = (*fsDbModelsStore)(nil)

func newFsDbModelsStore(projectPath string) fsDbModelsStore {
	return fsDbModelsStore{
		fsProjectItemsStore: newFileProjectItemsStore[datatug.DbModels, *datatug.DbModel, datatug.DbModel](
			path.Join(projectPath, storage.DbModelsFolder), storage.DbModelFileSuffix,
		),
	}
}

type fsDbModelsStore struct {
	fsProjectItemsStore[datatug.DbModels, *datatug.DbModel, datatug.DbModel]
}

func (s fsDbModelsStore) LoadDbModels(ctx context.Context, o ...datatug.StoreOption) (datatug.DbModels, error) {
	items, err := s.loadProjectItems(ctx, s.dirPath, o...)
	return items, err
}

func (s fsDbModelsStore) LoadDbModel(ctx context.Context, id string, o ...datatug.StoreOption) (*datatug.DbModel, error) {
	return s.loadProjectItem(ctx, s.dirPath, id, s.itemFileName(id), o...)
}

func (s fsDbModelsStore) SaveDbModel(ctx context.Context, DbModel *datatug.DbModel) error {
	return s.saveProjectItem(ctx, s.dirPath, DbModel)
}

func (s fsDbModelsStore) SaveDbModels(ctx context.Context, DbModels datatug.DbModels) error {
	return s.saveProjectItems(ctx, s.dirPath, DbModels)
}

func (s fsDbModelsStore) DeleteDbModel(ctx context.Context, id string) error {
	return s.deleteProjectItem(ctx, s.dirPath, id)
}
