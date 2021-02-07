package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/validation"
	"net/http"
)

// GetQueries returns list of project queries
func GetQueries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	projectID := q.Get(urlQueryParamProjectID)
	folder := q.Get(urlQueryParamFolder)
	v, err := api.GetQueries(projectID, folder)
	ReturnJSON(w, r, http.StatusOK, err, v)
}

// SaveQuery handles save query endpoint
func GetQuery(w http.ResponseWriter, r *http.Request) {
	params, err := getQueryRequestParams(r)
	if err != nil {
		handleError(err, w, r)
	}
	var query models.Query
	saveFunc := func(projectID string) error {
		return api.SaveQuery(params, query)
	}
	saveItem(w, r, &query, saveFunc)
}

// SaveQuery handles save query endpoint
func SaveQuery(w http.ResponseWriter, r *http.Request) {
	params, err := getQueryRequestParams(r)
	if err != nil {
		handleError(err, w, r)
	}
	var query models.Query
	saveFunc := func(projectID string) error {
		return api.SaveQuery(params, query)
	}
	saveItem(w, r, &query, saveFunc)
}

// DeleteQuery handles delete query endpoint
func DeleteQuery(w http.ResponseWriter, request *http.Request) {
	deleteItem(w, request, "entity", api.DeleteQuery)
}

func getQueryRequestParams(r *http.Request) (params api.QueryRequestParams, err error) {
	query := r.URL.Query()
	if params.Project = query.Get(urlQueryParamProjectID); params.Project == "" {
		err = validation.NewErrRequestIsMissingRequiredField(urlQueryParamProjectID)
		return
	}
	if params.Query = query.Get(urlQueryParamQuery); params.Query == "" {
		err = validation.NewErrRequestIsMissingRequiredField(urlQueryParamQuery)
		return
	}
	return
}
