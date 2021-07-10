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

var createProjectVerifyOptions = VerifyRequest{
	MinContentLength: int64(len(`{}`)),
	MaxContentLength: 1024,
	AuthRequired:     true,
}

// CreateProject creates project
func (ProjectAgentEndpoints) CreateProject(w http.ResponseWriter, r *http.Request) {
	request := dto.CreateProjectRequest{
		StoreID: r.URL.Query().Get("store"),
	}
	handle(w, r, &request, createProjectVerifyOptions, http.StatusOK, func(ctx context.Context) (response ResponseDTO, err error) {
		return api.CreateProject(ctx, request)
	})
}

// DeleteProject deletes project
func (ProjectAgentEndpoints) DeleteProject(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("Deletion of a DataTug project is not implemented at agent yet."))
}

// GetProjectSummary a handler to return project summary
func GetProjectSummary(w http.ResponseWriter, r *http.Request) {
	ref := newProjectRef(r.URL.Query())
	handle(w, r, &ref, createProjectVerifyOptions, http.StatusOK, func(ctx context.Context) (response ResponseDTO, err error) {
		return api.GetProjectSummary(ctx, ref)
	})
}
