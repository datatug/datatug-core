package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/dto"
	"net/http"
)

var getQueries = api.GetQueries
var getQuery = api.GetQuery

// GetQueries returns list of project queries
func GetQueries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	folder := q.Get(urlQueryParamFolder)
	ref := newProjectRef(r.URL.Query())
	v, err := getQueries(ref, folder)
	returnJSON(w, r, http.StatusOK, err, v)
}

// GetQuery returns query definition
func GetQuery(w http.ResponseWriter, r *http.Request) {
	params, err := getQueryRequestParams(r, urlQueryParamQuery)
	if err != nil {
		handleError(err, w, r)
		return
	}
	query, err := getQuery(params)
	if err != nil {
		handleError(err, w, r)
		return
	}
	returnJSON(w, r, http.StatusOK, err, query)
}

// CreateQuery handles create query endpoint
var CreateQuery = func(w http.ResponseWriter, r *http.Request) {
	var request dto.CreateQuery
	saveFunc := func(ref dto.ProjectItemRef) (interface{}, error) {
		return &request, api.CreateQuery(request)
	}
	saveItem(w, r, &request, saveFunc)
}

// UpdateQuery handles update query endpoint
func UpdateQuery(w http.ResponseWriter, r *http.Request) {
	var request dto.UpdateQuery
	saveFunc := func(ref dto.ProjectItemRef) (interface{}, error) {
		return &request, api.UpdateQuery(request)
	}
	saveItem(w, r, &request, saveFunc)
}

// CreateQueryFolder handles create query endpoint
func CreateQueryFolder(w http.ResponseWriter, r *http.Request) {
	var request dto.CreateFolder
	saveFunc := func(ref dto.ProjectItemRef) (interface{}, error) {
		return api.CreateQueryFolder(request)
	}
	saveItem(w, r, &request, saveFunc)
	return
}

// DeleteQuery handles delete query endpoint
var DeleteQuery = deleteProjItem(api.DeleteQuery)

// DeleteQueryFolder handles delete query folder endpoint
var DeleteQueryFolder = deleteProjItem(api.DeleteQueryFolder)

func getQueryRequestParams(r *http.Request, idParamName string) (ref dto.ProjectItemRef, err error) {
	query := r.URL.Query()
	ref = newProjectItemRef(query)
	if err = ref.Validate(); err != nil {
		return
	}
	return
}
