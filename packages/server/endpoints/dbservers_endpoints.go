package endpoints

import (
	"context"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"log"
	"net/http"
)

// AddDbServer adds a new DB server to project
func AddDbServer(w http.ResponseWriter, r *http.Request) {
	var projDbServer models.ProjDbServer
	saveFunc := func(ctx context.Context, ref dto.ProjectItemRef) (interface{}, error) {
		return projDbServer, api.AddDbServer(ctx, ref.ProjectRef, projDbServer)
	}
	saveItem(w, r, &projDbServer, saveFunc)
}

// GetDbServerSummary returns summary about environment
func GetDbServerSummary(w http.ResponseWriter, r *http.Request) {
	log.Println(r.Method, r.RequestURI)
	q := r.URL.Query()
	dbServer := models.ServerReference{
		Driver: q.Get("driver"),
		Host:   q.Get("host"),
	}
	ref := newProjectRef(q)
	summary, err := api.GetDbServerSummary(r.Context(), ref, dbServer)
	returnJSON(w, r, http.StatusOK, err, summary)
}

// DeleteDbServer removes a DB server from project
func DeleteDbServer(w http.ResponseWriter, r *http.Request) {
	log.Println(r.Method, r.RequestURI)
	q := r.URL.Query()
	var err error
	dbServer, err := newDbServerFromQueryParams(q)
	if err != nil {
		handleError(err, w, r)
		return
	}
	ref := newProjectRef(q)
	if err = api.DeleteDbServer(r.Context(), ref, dbServer); err != nil {
		handleError(err, w, r)
		return
	}
	returnJSON(w, r, http.StatusOK, err, nil)
}
