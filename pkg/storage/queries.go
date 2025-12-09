package storage

import (
	"context"

	"github.com/datatug/datatug-core/pkg/models"
)

// QueriesStore provides access to queries
type QueriesStore interface {
	ProjectStoreRef
	CreateQuery(ctx context.Context, query models.QueryDefWithFolderPath) (*models.QueryDefWithFolderPath, error)
	UpdateQuery(ctx context.Context, query models.QueryDef) (*models.QueryDefWithFolderPath, error)
	GetQuery(ctx context.Context, id string) (query *models.QueryDefWithFolderPath, err error)
	DeleteQuery(ctx context.Context, id string) (err error)
}
