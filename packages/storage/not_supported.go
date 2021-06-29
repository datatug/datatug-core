package storage

import (
	"errors"
	"github.com/datatug/datatug/packages/models"
)

// NotSupportedLoader is not supported
type NotSupportedLoader struct {
	NotSupportedQueriesLoader
}

var _ QueriesLoader = (*NotSupportedQueriesLoader)(nil)
var _ QueriesLoader = (*NotSupportedQueriesLoader)(nil)

// NotSupportedQueriesLoader is not supported
type NotSupportedQueriesLoader struct {
}

// LoadQueries is not supported
func (NotSupportedQueriesLoader) LoadQueries(folderPath string) (folder *models.QueryFolder, err error) {
	err = errNotSupported
	return
}

// LoadQuery is not supported
func (NotSupportedQueriesLoader) LoadQuery(queryID string) (query *models.QueryDef, err error) {
	err = errNotSupported
	return
}

// NotSupportedQuerySaver return not supported error
type NotSupportedQuerySaver struct {
}

var _ QuerySaver = (*NotSupportedQuerySaver)(nil)

var errNotSupported = errors.New("not supported")

// DeleteQuery is not supported
func (NotSupportedQuerySaver) DeleteQuery(queryID string) (err error) {
	return errNotSupported
}

// DeleteQueryFolder is not supported
func (NotSupportedQuerySaver) DeleteQueryFolder(path string) (err error) {
	return errNotSupported
}

// CreateQueryFolder is not supported
func (NotSupportedQuerySaver) CreateQueryFolder(path, id string) (folder *models.QueryFolder, err error) {
	err = errNotSupported
	return
}

// CreateQuery is not supported
func (NotSupportedQuerySaver) CreateQuery(query models.QueryDef) (err error) {
	return errNotSupported
}

// UpdateQuery is not supported
func (NotSupportedQuerySaver) UpdateQuery(query models.QueryDef) (err error) {
	return errNotSupported
}
