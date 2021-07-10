package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

// GetProjectFull returns full info about a project
func GetProjectFull(w http.ResponseWriter, r *http.Request) {
	ctx, err := GetContext(r)
	if err != nil {
		handleError(err, w, r)
	}
	ref := newProjectRef(r.URL.Query())
	project, err := api.GetProjectFull(ctx, ref)
	returnJSON(w, r, http.StatusOK, err, project)
}
