package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

// GetProjectFull returns full info about a project
func GetProjectFull(w http.ResponseWriter, r *http.Request) {
	ref := newProjectRef(r.URL.Query())
	project, err := api.GetProjectFull(r.Context(), ref)
	returnJSON(w, r, http.StatusOK, err, project)
}
