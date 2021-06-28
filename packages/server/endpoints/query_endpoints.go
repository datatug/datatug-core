package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/models"
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
func CreateQuery(w http.ResponseWriter, r *http.Request) {
	saveQuery(w, r, urlQueryParamID, api.CreateQuery)
}

// CreateQueryFolder handles create query endpoint
func CreateQueryFolder(w http.ResponseWriter, r *http.Request) {
	createQueryFolder(w, r)
}

// UpdateQuery handles update query endpoint
func UpdateQuery(w http.ResponseWriter, r *http.Request) {
	saveQuery(w, r, urlQueryParamQuery, api.UpdateQuery)
}

func saveQuery(w http.ResponseWriter, r *http.Request, idParamName string, save func(ref api.ProjectItemRef, query models.QueryDef) error) {
	var query models.QueryDef
	saveFunc := func(ref api.ProjectItemRef) (interface{}, error) {
		return query, save(ref, query)
	}
	saveItem(w, r, &query, saveFunc)
}

func createQueryFolder(w http.ResponseWriter, r *http.Request) {
	type params struct {
		Path string `json:"path"`
		ID   string `json:"id"`
	}
	var p params
	saveFunc := func(ref api.ProjectItemRef) (interface{}, error) {
		return api.CreateQueryFolder(ref.ProjectRef, p.Path, p.ID)
	}
	saveItem(w, r, &p, saveFunc)
	return
}

// DeleteQuery handles delete query endpoint
func DeleteQuery(w http.ResponseWriter, request *http.Request) {
	deleteItem(w, request, "id", api.DeleteQuery)
}

// DeleteQueryFolder handles delete query endpoint
func DeleteQueryFolder(w http.ResponseWriter, request *http.Request) {
	deleteItem(w, request, "id", api.DeleteQueryFolder)
}

func getQueryRequestParams(r *http.Request, idParamName string) (ref api.ProjectItemRef, err error) {
	query := r.URL.Query()
	ref = newProjectItemRef(query)
	if err = ref.Validate(); err != nil {
		return
	}
	return
}
