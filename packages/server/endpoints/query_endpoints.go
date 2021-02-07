package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/models"
	"net/http"
)

// GetQueries returns list of project queries
func GetQueries(w http.ResponseWriter, request *http.Request) {
	q := request.URL.Query()
	projectID := q.Get(urlQueryParamProjectID)
	folder := q.Get(urlQueryParamProjectID)
	v, err := api.GetQueries(projectID, folder)
	ReturnJSON(w, request, http.StatusOK, err, v)
}

// SaveQuery handles save query endpoint
func SaveQuery(w http.ResponseWriter, request *http.Request) {
	var query models.Query
	saveFunc := func(projectID string) error {
		return api.SaveQuery(projectID, query)
	}
	saveItem(w, request, &query, saveFunc)
}

// DeleteQuery handles delete query endpoint
func DeleteQuery(w http.ResponseWriter, request *http.Request) {
	deleteItem(w, request, "entity", api.DeleteQuery)
}
