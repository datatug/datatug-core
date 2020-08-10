package endpoints

import (
	"github.com/datatug/datatug/packages/store"
	"net/http"
)

// GetProjects returns list of projects
func GetProjects(w http.ResponseWriter, request *http.Request) {
	projectBriefs, err := store.Current.GetProjects()
	ReturnJSON(w, request, http.StatusOK, err, projectBriefs)
}
