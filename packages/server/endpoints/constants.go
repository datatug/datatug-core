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

func fillProjectItemRef(ref *dto.ProjectItemRef, query url.Values, idParamName string) {
	ref.ProjectRef = newProjectRef(query)
	ref.ID = query.Get(idParamName)
}

func newProjectItemRef(query url.Values, idParamName string) (ref dto.ProjectItemRef) {
	if idParamName == "" {
		idParamName = urlParamID
	}
	fillProjectItemRef(&ref, query, idParamName)
	return
}
