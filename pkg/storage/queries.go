package storage

import (
	"context"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// QueriesStore provides access to queries
type QueriesStore interface {
	ProjectStoreRef
	CreateQuery(ctx context.Context, query datatug.QueryDefWithFolderPath) (*datatug.QueryDefWithFolderPath, error)
	UpdateQuery(ctx context.Context, query datatug.QueryDef) (*datatug.QueryDefWithFolderPath, error)
	GetQuery(ctx context.Context, id string) (query *datatug.QueryDefWithFolderPath, err error)
	DeleteQuery(ctx context.Context, id string) (err error)
}
