package endpoints

import (
	"context"
	"net/http"
)

// ProjectEndpoints defines project endpoints
type ProjectEndpoints interface {
	CreateProject(w http.ResponseWriter, r *http.Request)
	DeleteProject(w http.ResponseWriter, r *http.Request)
}

var Context = func(r *http.Request) (context.Context, error) {
	return r.Context(), nil
}
