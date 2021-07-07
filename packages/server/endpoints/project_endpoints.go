package endpoints

import (
	"context"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/dto"
	"net/http"
)

var _ ProjectEndpoints = (*ProjectAgentEndpoints)(nil)

// ProjectAgentEndpoints defines project endpoints
type ProjectAgentEndpoints struct {
}

// CreateProject creates project
func (ProjectAgentEndpoints) CreateProject(w http.ResponseWriter, r *http.Request) {
	request := dto.CreateProjectRequest{
		StoreID: r.URL.Query().Get("store"),
	}
	verifyOptions := verifyRequestOptions{
		minContentLength: len(`{"title"":""}`),
		maxContentLength: 1024,
		authRequired:     true,
	}
	handle(w, r, request, verifyOptions, http.StatusOK, func(ctx context.Context) (response ResponseDTO, err error) {
		return api.CreateProject(r.Context(), request)
	})
}

// DeleteProject deletes project
func (ProjectAgentEndpoints) DeleteProject(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("Deletion of a DataTug project is not implemented at agent yet."))
}

// GetProjectSummary a handler to return project summary
func GetProjectSummary(w http.ResponseWriter, r *http.Request) {
	ctx, err := Context(w, r)
	if err != nil {
		handleError(err, w, r)
	}
	ref := newProjectRef(r.URL.Query())
	projectSummary, err := api.GetProjectSummary(ctx, ref)
	returnJSON(w, r, http.StatusOK, err, projectSummary)
}
