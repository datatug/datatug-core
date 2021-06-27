package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

// GetEnvironmentSummary returns summary about environment
func GetEnvironmentSummary(w http.ResponseWriter, request *http.Request) {

	ref := newProjectItemRef(request.URL.Query())
	summary, err := api.GetEnvironmentSummary(ref)
	returnJSON(w, request, http.StatusOK, err, summary)
}
