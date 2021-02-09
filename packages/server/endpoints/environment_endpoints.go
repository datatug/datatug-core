package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

// GetEnvironmentSummary returns summary about environment
func GetEnvironmentSummary(w http.ResponseWriter, request *http.Request) {
	q := request.URL.Query()
	envID := q.Get("env")
	projID := q.Get("proj")
	summary, err := api.GetEnvironmentSummary(projID, envID)
	returnJSON(w, request, http.StatusOK, err, summary)
}
