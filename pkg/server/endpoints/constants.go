package endpoints

import (
	"net/url"

	"github.com/datatug/datatug-core/pkg/dto"
)

const (
	urlParamID          = "id"
	urlParamStoreID     = "storage"
	urlParamProjectID   = "project"
	urlParamRecordsetID = "recordset"
	urlParamDataID      = "data"
)

func fillProjectRef(ref *dto.ProjectRef, q url.Values) {
	ref.StoreID = q.Get(urlParamStoreID)
	if ref.StoreID == "" {
		ref.StoreID = "firestore"
	}
	ref.ProjectID = q.Get(urlParamProjectID)
}

func newProjectRef(q url.Values) (ref dto.ProjectRef) {
	fillProjectRef(&ref, q)
	return
}

func fillProjectItemRef(ref *dto.ProjectItemRef, q url.Values, idParamName string) {
	fillProjectRef(&ref.ProjectRef, q)
	ref.ID = q.Get(idParamName)
}

func newProjectItemRef(q url.Values, idParamName string) (ref dto.ProjectItemRef) {
	if idParamName == "" {
		idParamName = urlParamID
	}
	fillProjectItemRef(&ref, q, idParamName)
	return
}
