package endpoints

import (
	"github.com/datatug/datatug/packages/dto"
	"net/url"
)

const (
	urlParamID          = "id"
	urlParamStoreID     = "storage"
	urlParamProjectID   = "project"
	urlParamRecordsetID = "recordset"
	urlParamDataID      = "data"
)

func newProjectRef(query url.Values) dto.ProjectRef {
	return dto.ProjectRef{
		StoreID:   query.Get(urlParamStoreID),
		ProjectID: query.Get(urlParamProjectID),
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
