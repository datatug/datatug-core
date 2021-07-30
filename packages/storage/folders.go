package storage

import (
	"context"
	"github.com/datatug/datatug/packages/models"
)

type CreateFolderRequest struct {
	Name string
	Path string
	Note string
}

func (v CreateFolderRequest) Validate() error {
	return nil
}

type FoldersStore interface {
	CreateFolder(ctx context.Context, request CreateFolderRequest) (folder *models.Folder, err error)
	GetFolder(ctx context.Context, path string) (folder *models.Folder, err error)
	DeleteFolder(ctx context.Context, path string) (err error)
}
