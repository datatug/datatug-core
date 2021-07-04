package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

// GetEnvironmentSummary returns summary about environment
func GetEnvironmentSummary(w http.ResponseWriter, r *http.Request) {

	ref := newProjectItemRef(r.URL.Query())
	summary, err := api.GetEnvironmentSummary(r.Context(), ref)
	returnJSON(w, r, http.StatusOK, err, summary)
}
