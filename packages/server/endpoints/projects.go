package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

// GetProjects returns list of projects
func GetProjects(w http.ResponseWriter, r *http.Request) {
	storeID := r.URL.Query().Get(urlQueryParamStoreID)
	projectBriefs, err := api.GetProjects(r.Context(), storeID)
	returnJSON(w, r, http.StatusOK, err, projectBriefs)
}
