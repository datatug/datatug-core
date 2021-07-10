package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

// getEnvironmentSummary returns summary about environment
func getEnvironmentSummary(w http.ResponseWriter, r *http.Request) {
	ctx, err := getContextFromRequest(r)
	if err != nil {
		handleError(err, w, r)
	}
	ref := newProjectItemRef(r.URL.Query(), "")
	summary, err := api.GetEnvironmentSummary(ctx, ref)
	returnJSON(w, r, http.StatusOK, err, summary)
}
