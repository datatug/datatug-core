package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

// GetEnvironmentSummary returns summary about environment
func GetEnvironmentSummary(w http.ResponseWriter, r *http.Request) {
	ctx, err := GetContextFromRequest(r)
	if err != nil {
		handleError(err, w, r)
	}
	ref := newProjectItemRef(r.URL.Query(), "")
	summary, err := api.GetEnvironmentSummary(ctx, ref)
	returnJSON(w, r, http.StatusOK, err, summary)
}
