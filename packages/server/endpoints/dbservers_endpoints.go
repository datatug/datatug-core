package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/models"
	"log"
	"net/http"
)

// AddDbServer adds a new DB server to project
func AddDbServer(w http.ResponseWriter, r *http.Request) {
	log.Println(r.Method, r.RequestURI)
	q := r.URL.Query()
	projID := q.Get("proj")
	dbServer, err := newDbServerFromQueryParams(q)
	if err != nil {
		handleError(err, w, r)
		return
	}
	if err = api.AddDbServer(projID, dbServer); err != nil {
		handleError(err, w, r)
		return
	}
	summary, err := api.GetDbServerSummary(projID, dbServer)
	ReturnJSON(w, r, http.StatusOK, err, summary)
}

// GetDbServerSummary returns summary about environment
func GetDbServerSummary(w http.ResponseWriter, request *http.Request) {
	log.Println(request.Method, request.RequestURI)
	q := request.URL.Query()
	projID := q.Get("proj")
	dbServer := models.DbServer{
		Driver: q.Get("driver"),
		Host:   q.Get("host"),
	}
	summary, err := api.GetDbServerSummary(projID, dbServer)
	ReturnJSON(w, request, http.StatusOK, err, summary)
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
	projID := q.Get("proj")
	if err = api.DeleteDbServer(projID, dbServer); err != nil {
		handleError(err, w, r)
		return
	}
	ReturnJSON(w, r, http.StatusOK, err, nil)
}
