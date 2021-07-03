package storage

import (
	"context"
	"github.com/datatug/datatug/packages/models"
)

// QueriesStore provides access to queries
type QueriesStore interface {
	ProjectStoreRef
	Query(id string) QueryStore
	LoadQueries(ctx context.Context, folderPath string) (folder *models.QueryFolder, err error)
	DeleteQueryFolder(ctx context.Context, path string) (err error)
	CreateQueryFolder(ctx context.Context, path, id string) (folder *models.QueryFolder, err error)
	CreateQuery(ctx context.Context, query models.QueryDef) (err error)
}

// QueryStore provides access to a specific query
type QueryStore interface {
	ID() string

	LoadQuery(ctx context.Context) (query *models.QueryDef, err error)
	DeleteQuery(ctx context.Context) (err error)
	UpdateQuery(ctx context.Context, query models.QueryDef) (err error)
}
