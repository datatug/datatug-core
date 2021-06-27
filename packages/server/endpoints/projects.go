package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

// GetProjects returns list of projects
func GetProjects(w http.ResponseWriter, request *http.Request) {
	storeID := request.URL.Query().Get(urlQueryParamStoreID)
	projectBriefs, err := api.GetProjects(storeID)
	returnJSON(w, request, http.StatusOK, err, projectBriefs)
}
