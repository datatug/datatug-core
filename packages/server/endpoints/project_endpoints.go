package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

var _ ProjectEndpoints = (*ProjectAgentEndpoints)(nil)

// ProjectAgentEndpoints defines project endpoints
type ProjectAgentEndpoints struct {
}

// CreateProject creates project
func (ProjectAgentEndpoints) CreateProject(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("Creation of a new DataTug project is not implemented at agent yet. For now use DataTug CLI to create a new project."))
}

// DeleteProject deletes project
func (ProjectAgentEndpoints) DeleteProject(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("Deletion of a DataTug project is not implemented at agent yet."))
}

// GetProjectSummary a handler to return project summary
func GetProjectSummary(w http.ResponseWriter, r *http.Request) {
	ctx, err := Context(r)
	if err != nil {
		handleError(err, w, r)
	}
	ref := newProjectRef(r.URL.Query())
	projectSummary, err := api.GetProjectSummary(ctx, ref)
	returnJSON(w, r, http.StatusOK, err, projectSummary)
}
