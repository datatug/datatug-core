package endpoints

import (
	"context"
	"net/http"

	"github.com/datatug/datatug-core/pkg/api"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/sneat-co/sneat-go-core/apicore"
)

//var _ ProjectEndpoints = (*ProjectAgentEndpoints)(nil)

// ProjectAgentEndpoints defines project endpoints
type ProjectAgentEndpoints struct {
}

// createProject creates project
func (ProjectAgentEndpoints) createProject(w http.ResponseWriter, r *http.Request) {
	request := dto.CreateProjectRequest{
		StoreID: r.URL.Query().Get("store"),
	}
	var worker = func(ctx context.Context) (responseDTO apicore.ResponseDTO, err error) {
		return api.CreateProject(ctx, request)
	}
	verifyOptions := VerifyRequest{
		MinContentLength: int64(len(`{"title":""}`)),
		MaxContentLength: 1024,
		AuthRequired:     true,
	}
	handle(w, r, &request, verifyOptions, http.StatusOK, getContextFromRequest, worker)
}

// deleteProject deletes project
func (ProjectAgentEndpoints) deleteProject(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write([]byte("Deletion of a DataTug project is not implemented at agent yet."))
}

// getProjectSummary a handler to return project summary
func getProjectSummary(w http.ResponseWriter, r *http.Request) {
	ref := newProjectRef(r.URL.Query())
	worker := func(ctx context.Context) (response apicore.ResponseDTO, err error) {
		return api.GetProjectSummary(ctx, ref)
	}
	handle(w, r, &ref, VerifyRequest{AuthRequired: true}, http.StatusOK, getContextFromRequest, worker)
}
