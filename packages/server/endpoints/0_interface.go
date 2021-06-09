package endpoints

import "net/http"

type ProjectEndpoints interface {
	CreateProject(w http.ResponseWriter, r *http.Request)
	DeleteProject(w http.ResponseWriter, r *http.Request)
}
