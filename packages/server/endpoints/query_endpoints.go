package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/validation"
	"net/http"
)

var getQueries = api.GetQueries
var getQuery = api.GetQuery

// GetQueries returns list of project queries
func GetQueries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	projectID := q.Get(urlQueryParamProjectID)
	folder := q.Get(urlQueryParamFolder)
	v, err := getQueries(projectID, folder)
	returnJSON(w, r, http.StatusOK, err, v)
}

// SaveQuery handles save query endpoint
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

func saveQuery(w http.ResponseWriter, r *http.Request, idParamName string, save func(params api.QueryRequestParams, query models.QueryDef) error) {
	params, err := getQueryRequestParams(r, idParamName)
	if err != nil {
		handleError(err, w, r)
	}
	var query models.QueryDef
	saveFunc := func(projectID string) (interface{}, error) {
		return query, save(params, query)
	}
	saveItem(w, r, &query, saveFunc)
}

func createQueryFolder(w http.ResponseWriter, r *http.Request) {
	type params struct {
		Path string `json:"path"`
		ID   string `json:"id"`
	}
	var p params
	saveFunc := func(projectID string) (interface{}, error) {
		return api.CreateQueryFolder(projectID, p.Path, p.ID)
	}
	saveItem(w, r, &p, saveFunc)
	return
}

// DeleteQuery handles delete query endpoint
func DeleteQuery(w http.ResponseWriter, request *http.Request) {
	deleteItem(w, request, "id", api.DeleteQuery)
}

// DeleteQuery handles delete query endpoint
func DeleteQueryFolder(w http.ResponseWriter, request *http.Request) {
	deleteItem(w, request, "id", api.DeleteQueryFolder)
}

func getQueryRequestParams(r *http.Request, idParamName string) (params api.QueryRequestParams, err error) {
	query := r.URL.Query()
	if params.Project = query.Get(urlQueryParamProjectID); params.Project == "" {
		err = validation.NewErrRequestIsMissingRequiredField(urlQueryParamProjectID)
		return
	}
	if params.Query = query.Get(idParamName); params.Query == "" {
		err = validation.NewErrRequestIsMissingRequiredField(urlQueryParamQuery)
		return
	}
	return
}
