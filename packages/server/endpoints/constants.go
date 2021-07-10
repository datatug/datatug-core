package endpoints

import (
	"github.com/datatug/datatug/packages/dto"
	"net/url"
)

const (
	urlParamID               = "id"
	urlQueryParamStoreID     = "storage"
	urlQueryParamProjectID   = "project"
	urlQueryParamRecordsetID = "recordset"
	urlQueryParamDataID      = "data"
)

func newProjectRef(query url.Values) dto.ProjectRef {
	return dto.ProjectRef{
		StoreID:   query.Get(urlQueryParamStoreID),
		ProjectID: query.Get(urlQueryParamProjectID),
	}
}
func newProjectItemRef(query url.Values, idParamName string) dto.ProjectItemRef {
	if idParamName == "" {
		idParamName = urlParamID
	}
	return dto.ProjectItemRef{
		ProjectRef: newProjectRef(query),
		ID:         query.Get(idParamName),
	}
}
