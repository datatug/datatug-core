package filestore

import (
	"context"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
)

var _ storage.FoldersStore = (*fsFoldersStore)(nil)

type fsFoldersStore struct {
	fsProjectStore
}

func (f fsFoldersStore) CreateFolder(ctx context.Context, path, name string) (err error) {
	panic("implement me")
}

func (f fsFoldersStore) GetFolder(ctx context.Context, path string) (folder *models.Folder, err error) {
	panic("implement me")
}

func (f fsFoldersStore) DeleteFolder(ctx context.Context, path string) (err error) {
	panic("implement me")
}

