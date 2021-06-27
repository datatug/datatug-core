package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/url"
)

const (
	urlQueryParamID          = "id"
	urlQueryParamStoreID   = "store"
	urlQueryParamProjectID   = "project"
	urlQueryParamRecordsetID = "recordset"
	urlQueryParamDataID      = "data"
	urlQueryParamFolder      = "folder"
	urlQueryParamQuery      = "query"
)

func newProjectRef(query url.Values) api.ProjectRef {
	return api.ProjectRef{
		StoreID: query.Get(urlQueryParamStoreID),
		ProjectID: query.Get(urlQueryParamProjectID),
	}
}
func newProjectItemRef(query url.Values) api.ProjectItemRef {
	return api.ProjectItemRef{
		ProjectRef: newProjectRef(query),
		ID: query.Get(urlQueryParamID),
	}
}