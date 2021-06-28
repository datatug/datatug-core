package store

import (
	"errors"
	"github.com/datatug/datatug/packages/models"
)

// NotSupportedQuerySaver return not supported error
type NotSupportedQuerySaver struct {
}

var errNotSupported = errors.New("not supported")

// DeleteQuery is not supported
func (NotSupportedQuerySaver) DeleteQuery(projID, queryID string) (err error) {
	return errNotSupported
}

// DeleteQueryFolder is not supported
func (NotSupportedQuerySaver) DeleteQueryFolder(projID, path string) (err error) {
	return errNotSupported
}

// CreateQueryFolder is not supported
func (NotSupportedQuerySaver) CreateQueryFolder(projID, path, id string) (folder models.QueryFolder, err error) {
	err = errNotSupported
	return
}

// CreateQuery is not supported
func (NotSupportedQuerySaver) CreateQuery(projID string, query models.QueryDef) (err error) {
	return errNotSupported
}

// UpdateQuery is not supported
func (NotSupportedQuerySaver) UpdateQuery(projID string, query models.QueryDef) (err error) {
	return errNotSupported
}
