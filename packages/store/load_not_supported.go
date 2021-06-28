package store

import "github.com/datatug/datatug/packages/models"

//var _ Loader = (*NotSupportedLoader)(nil)

// NotSupportedLoader is not supported
type NotSupportedLoader struct {
	NotSupportedQueriesLoader
}

var _ QueriesLoader = (*NotSupportedQueriesLoader)(nil)

// NotSupportedQueriesLoader is not supported
type NotSupportedQueriesLoader struct {
}

// LoadQueries is not supported
func (NotSupportedQueriesLoader) LoadQueries(projectID, folderPath string) (folder *models.QueryFolder, err error) {
	err = errNotSupported
	return
}

// LoadQuery is not supported
func (NotSupportedQueriesLoader) LoadQuery(projectID, queryID string) (query *models.QueryDef, err error) {
	err = errNotSupported
	return
}
