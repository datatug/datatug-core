package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

// getProjects returns list of projects
func getProjects(w http.ResponseWriter, r *http.Request) {
	storeID := r.URL.Query().Get(urlParamStoreID)
	ctx, err := getContextFromRequest(r)
	if err != nil {
		handleError(err, w, r)
	}
	projectBriefs, err := api.GetProjects(ctx, storeID)
	returnJSON(w, r, http.StatusOK, err, projectBriefs)
}
