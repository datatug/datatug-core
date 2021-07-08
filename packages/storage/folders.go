package storage

import (
	"context"
	"github.com/datatug/datatug/packages/models"
)

type FoldersStore interface {
	CreateFolder(ctx context.Context, path, name string) (err error)
	GetFolder(ctx context.Context, path string) (folder *models.Folder, err error)
	DeleteFolder(ctx context.Context, path string) (err error)
}
