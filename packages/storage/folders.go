package storage

import (
	"context"
	"github.com/datatug/datatug/packages/models"
)

type FolderStore interface {
	CreateFolder(ctx context.Context, name string) (err error)
	GetFolder(ctx context.Context) (folder *models.Folder, err error)
	DeleteFolder(ctx context.Context) (err error)
}
