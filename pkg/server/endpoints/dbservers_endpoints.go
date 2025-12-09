package endpoints

import (
	"context"
	"log"
	"net/http"

	"github.com/datatug/datatug-core/pkg/api"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/datatug/datatug-core/pkg/models"
	"github.com/sneat-co/sneat-go-core/apicore"
)

// addDbServer adds a new DB server to project
func addDbServer(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectRef
	var projDbServer models.ProjDbServer
	saveFunc := func(ctx context.Context) (apicore.ResponseDTO, error) {
		return projDbServer, api.AddDbServer(ctx, ref, projDbServer)
	}
	createProjectItem(w, r, &ref, &projDbServer, saveFunc)
}

// getDbServerSummary returns summary about environment
func getDbServerSummary(w http.ResponseWriter, r *http.Request) {
	log.Println(r.Method, r.RequestURI)
	q := r.URL.Query()
	dbServer := models.ServerReference{
		Driver: q.Get("driver"),
		Host:   q.Get("host"),
	}
	ref := newProjectRef(q)
	ctx, err := getContextFromRequest(r)
	if err != nil {
		handleError(err, w, r)
	}
	summary, err := api.GetDbServerSummary(ctx, ref, dbServer)
	returnJSON(w, r, http.StatusOK, err, summary)
}

// deleteDbServer removes a DB server from project
func deleteDbServer(w http.ResponseWriter, r *http.Request) {
	log.Println(r.Method, r.RequestURI)
	q := r.URL.Query()
	var err error
	dbServer, err := newDbServerFromQueryParams(q)
	if err != nil {
		handleError(err, w, r)
		return
	}
	ctx, err := getContextFromRequest(r)
	if err != nil {
		handleError(err, w, r)
	}
	ref := newProjectRef(q)
	if err = api.DeleteDbServer(ctx, ref, dbServer); err != nil {
		handleError(err, w, r)
		return
	}
	returnJSON(w, r, http.StatusOK, err, nil)
}
