package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

// GetProjects returns list of projects
func GetProjects(w http.ResponseWriter, r *http.Request) {
	storeID := r.URL.Query().Get(urlQueryParamStoreID)
	ctx, err := Context(r)
	if err != nil {
		handleError(err, w, r)
	}
	projectBriefs, err := api.GetProjects(ctx, storeID)
	returnJSON(w, r, http.StatusOK, err, projectBriefs)
}
