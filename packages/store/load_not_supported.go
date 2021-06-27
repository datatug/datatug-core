package store

import "github.com/datatug/datatug/packages/models"

//var _ Loader = (*NotSupportedLoader)(nil)

type NotSupportedLoader struct {
	NotSupportedQueriesLoader
}

var _ QueriesLoader = (*NotSupportedQueriesLoader)(nil)

type NotSupportedQueriesLoader struct {
}

func (NotSupportedQueriesLoader) LoadQueries(projectID, folderPath string) (folder *models.QueryFolder, err error) {
	err = errNotSupported
	return
}

func (NotSupportedQueriesLoader) LoadQuery(projectID, queryID string) (query *models.QueryDef, err error) {
	err = errNotSupported
	return
}
