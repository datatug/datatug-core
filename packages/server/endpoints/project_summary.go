package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

// GetProjectSummary a handler to return project summary
func GetProjectSummary(w http.ResponseWriter, request *http.Request) {
	id := request.URL.Query().Get(urlQueryParamID)
	projectSummary, err := api.GetProjectSummary(id)
	returnJSON(w, request, http.StatusOK, err, projectSummary)
}
