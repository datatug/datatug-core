package storage

import "github.com/datatug/datatug/packages/models"

// QueriesStore provides access to queries
type QueriesStore interface {
	ProjectStoreRef
	Query(id string) QueryStore
	LoadQueries(folderPath string) (folder *models.QueryFolder, err error)
	DeleteQueryFolder(path string) (err error)
	CreateQueryFolder(path, id string) (folder *models.QueryFolder, err error)
	CreateQuery(query models.QueryDef) (err error)
}

// QueryStore provides access to a specific query
type QueryStore interface {
	ID() string

	LoadQuery() (query *models.QueryDef, err error)
	DeleteQuery() (err error)
	UpdateQuery(query models.QueryDef) (err error)
}
