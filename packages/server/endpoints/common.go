package endpoints

import (
	"context"
	"github.com/datatug/datatug/packages/dto"
	"net/http"
)

func deleteProjItem(del func(ctx context.Context, ref dto.ProjectItemRef) error) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		ref := newProjectItemRef(query, "")
		ctx, err := getContextFromRequest(r)
		if err != nil {
			handleError(err, w, r)
		}
		err = del(ctx, ref)
		returnJSON(w, r, http.StatusOK, err, true)
	}
}

func createProjectItem(
	w http.ResponseWriter,
	r *http.Request,
	ref *dto.ProjectRef,
	requestDTO RequestDTO,
	f func(ctx context.Context) (responseDTO ResponseDTO, err error),
) {
	q := r.URL.Query()
	ref.StoreID = q.Get(urlParamStoreID)
	ref.ProjectID = q.Get(urlParamProjectID)

	handle(w, r, requestDTO, VerifyRequest{
		AuthRequired:     true,
		MinContentLength: 0,
		MaxContentLength: 1024 * 1024,
	}, http.StatusCreated, getContextFromRequest, f)
}

func saveProjectItem(
	w http.ResponseWriter, r *http.Request,
	ref *dto.ProjectItemRef,
	requestDTO RequestDTO,
	f func(ctx context.Context) (responseDTO ResponseDTO, err error),
) {
	fillProjectItemRef(ref, r.URL.Query(), "")
	handle(w, r, requestDTO, VerifyRequest{
		AuthRequired:     true,
		MinContentLength: 0,
		MaxContentLength: 1024 * 1024,
	}, http.StatusCreated, getContextFromRequest, f)
}

func getProjectItem(
	w http.ResponseWriter, r *http.Request,
	ref *dto.ProjectItemRef,
	requestDTO RequestDTO,
	f func(ctx context.Context) (responseDTO ResponseDTO, err error),
) {
	fillProjectItemRef(ref, r.URL.Query(), "")
	handle(w, r, requestDTO, VerifyRequest{
		AuthRequired: true,
	}, http.StatusCreated, getContextFromRequest, f)
}
