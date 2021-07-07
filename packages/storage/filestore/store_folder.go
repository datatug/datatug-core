package filestore

import (
	"context"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
)

type fsFolderStore struct {
	folderID string
	fsProjectStore
}

func (f fsFolderStore) CreateFolder(ctx context.Context, name string) (err error) {
	panic("implement me")
}

func (f fsFolderStore) GetFolder(ctx context.Context) (folder *models.Folder, err error) {
	panic("implement me")
}

func (f fsFolderStore) DeleteFolder(ctx context.Context) (err error) {
	panic("implement me")
}

var _ storage.FolderStore = (*fsFolderStore)(nil)


