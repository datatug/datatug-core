package endpoints

import (
	"github.com/datatug/datatug/packages/dto"
	"net/url"
)

const (
	urlQueryParamID          = "id"
	urlQueryParamStoreID     = "storage"
	urlQueryParamProjectID   = "project"
	urlQueryParamRecordsetID = "recordset"
	urlQueryParamDataID      = "data"
	urlQueryParamFolder      = "folder"
	urlQueryParamQuery       = "query"
)

func newProjectRef(query url.Values) dto.ProjectRef {
	return dto.ProjectRef{
		StoreID:   query.Get(urlQueryParamStoreID),
		ProjectID: query.Get(urlQueryParamProjectID),
	}
}
func newProjectItemRef(query url.Values) dto.ProjectItemRef {
	return dto.ProjectItemRef{
		ProjectRef: newProjectRef(query),
		ID:         query.Get(urlQueryParamID),
	}
}
