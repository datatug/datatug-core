package endpoints

import (
	"net/http"

	"github.com/datatug/datatug-core/pkg/api"
)

// getProjectFull returns full info about a project
func getProjectFull(w http.ResponseWriter, r *http.Request) {
	ctx, err := getContextFromRequest(r)
	if err != nil {
		handleError(err, w, r)
	}
	ref := newProjectRef(r.URL.Query())
	project, err := api.GetProjectFull(ctx, ref)
	returnJSON(w, r, http.StatusOK, err, project)
}
