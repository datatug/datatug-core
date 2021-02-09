package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

// GetProjectFull returns full info about a project
func GetProjectFull(writer http.ResponseWriter, request *http.Request) {
	projectID := request.URL.Query().Get(urlQueryParamID)
	project, err := api.GetProjectFull(projectID)
	returnJSON(writer, request, http.StatusOK, err, project)
}
