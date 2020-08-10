package endpoints

import (
	"fmt"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/validation"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// GetServerDatabases returns databases hosted at server
func GetServerDatabases(w http.ResponseWriter, r *http.Request) {
	log.Println(r.Method, r.RequestURI)
	q := r.URL.Query()
	request := api.GetServerDatabasesRequest{
		Project:     q.Get("proj"),
		Environment: q.Get("env"),
	}
	var err error
	if request.DbServer, err = newDbServerFromQueryParams(q); err != nil {
		handleError(err, w, r)
		return
	}
	databases, err := api.GetServerDatabases(request)
	ReturnJSON(w, r, http.StatusOK, err, databases)
}

func newDbServerFromQueryParams(query url.Values) (dbServer models.DbServer, err error) {
	dbServer.Driver = query.Get("driver")
	dbServer.Host = query.Get("host")
	if port := strings.TrimSpace(query.Get("port")); port != "" {
		if dbServer.Port, err = strconv.Atoi(port); err != nil {
			err = validation.NewBadRequestError(fmt.Errorf("port parameter is not a number: %w", err))
			return
		}
	}
	return
}
