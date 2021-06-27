package store

import (
	"errors"
	"github.com/datatug/datatug/packages/models"
)

type NotSupportedQuerySaver struct {
}

var errNotSupported = errors.New("not supported")

func (NotSupportedQuerySaver) DeleteQuery(projID, queryID string) (err error) {
	return errNotSupported
}

func (NotSupportedQuerySaver) DeleteQueryFolder(projID, path string) (err error) {
	return errNotSupported
}

func (NotSupportedQuerySaver) CreateQueryFolder(projID, path, id string) (folder models.QueryFolder, err error) {
	err = errNotSupported
	return
}

func (NotSupportedQuerySaver) CreateQuery(projID string, query models.QueryDef) (err error) {
	return errNotSupported
}

func (NotSupportedQuerySaver) UpdateQuery(projID string, query models.QueryDef) (err error) {
	return errNotSupported
}
