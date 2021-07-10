package endpoints

import (
	"context"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/dto"
	"net/http"
)

//var getQueries = api.GetQueries
var getQuery = api.GetQuery

//// GetQueries returns list of project queries
//func GetQueries(w http.ResponseWriter, r *http.Request) {
//	q := r.URL.Query()
//	folder := q.Get(urlQueryParamFolder)
//	ref := newProjectRef(r.URL.Query())
//	ctx, err := GetContext(r)
//	if err != nil {
//		handleError(err, w, r)
//	}
//	v, err := getQueries(ctx, ref, folder)
//	returnJSON(w, r, http.StatusOK, err, v)
//}

// GetQuery returns query definition
func GetQuery(w http.ResponseWriter, r *http.Request) {
	params, err := getQueryRequestParams(r, urlQueryParamQuery)
	if err != nil {
		handleError(err, w, r)
		return
	}
	ctx, err := GetContext(r)
	if err != nil {
		handleError(err, w, r)
	}
	query, err := getQuery(ctx, params)
	if err != nil {
		handleError(err, w, r)
		return
	}
	returnJSON(w, r, http.StatusOK, err, query)
}

// CreateQuery handles create query endpoint
var CreateQuery = func(w http.ResponseWriter, r *http.Request) {
	var request dto.CreateQuery
	saveFunc := func(ctx context.Context, ref dto.ProjectItemRef) (interface{}, error) {
		return api.CreateQuery(ctx, request)
	}
	saveItem(w, r, &request, saveFunc)
}

// UpdateQuery handles update query endpoint
func UpdateQuery(w http.ResponseWriter, r *http.Request) {
	var request dto.UpdateQuery
	saveFunc := func(ctx context.Context, ref dto.ProjectItemRef) (interface{}, error) {
		return api.UpdateQuery(ctx, request)
	}
	saveItem(w, r, &request, saveFunc)
}

// DeleteQuery handles delete query endpoint
var DeleteQuery = deleteProjItem(api.DeleteQuery)

func getQueryRequestParams(r *http.Request, idParamName string) (ref dto.ProjectItemRef, err error) {
	query := r.URL.Query()
	ref = newProjectItemRef(query)
	if err = ref.Validate(); err != nil {
		return
	}
	return
}
