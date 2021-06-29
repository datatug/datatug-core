package storage

import "github.com/datatug/datatug/packages/models"

type QueriesStore interface {
	Loader() QueriesLoader
	Saver() QuerySaver
}

// QueriesLoader loads queries
type QueriesLoader interface {
	// LoadQueries loads tree of queries
	LoadQueries(folderPath string) (folder *models.QueryFolder, err error)

	//
	LoadQuery(queryID string) (query *models.QueryDef, err error)
}

// QuerySaver saves queries
type QuerySaver interface {
	DeleteQuery(queryID string) (err error)
	DeleteQueryFolder(path string) (err error)
	CreateQueryFolder(path, id string) (folder *models.QueryFolder, err error)
	CreateQuery(query models.QueryDef) (err error)
	UpdateQuery(query models.QueryDef) (err error)
}
