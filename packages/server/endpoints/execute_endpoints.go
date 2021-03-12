package endpoints

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/execute"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
	"net/http"
	"strconv"
	"strings"
)

// ExecuteCommandsHandler handler for execute command endpoint
func ExecuteCommandsHandler(w http.ResponseWriter, r *http.Request) {

	var executeRequest execute.Request

	executeRequest.Project = r.URL.Query().Get("project")

	switch r.Method {
	//case "GET":
	//	q := r.URL.ExecuteSingle()
	//	executeRequest.ID = q.Get("id")
	//	executeRequest.Project = q.Get("p")
	//	env := q.Get("env")
	//	db := q.Get("db")
	//	executeRequest.Commands = append(executeRequest.Commands,
	//		execute.RequestCommand{
	//			Env:  env,
	//			DB:   db,
	//			Text: q.Get("q1"),
	//		},
	//	)
	case "POST":
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&executeRequest); err != nil {
			err = fmt.Errorf("%w: failed to decode request body", validation.NewBadRequestError(err))
			handleError(err, w, r)
			return
		}
	default:
		handleError(validation.NewBadRequestError(errors.New("only POST requests are supported for this endpoint")), w, r)
		return
	}

	response, err := api.ExecuteCommands(executeRequest)
	returnJSON(w, r, http.StatusOK, err, response)
}

// ExecuteSelectHandler executes select command
func ExecuteSelectHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	var limit int64
	var err error
	if v := query.Get("limit"); v == "" {
		limit = -1
	} else if limit, err = strconv.ParseInt(v, 10, 0); err != nil {
		err = validation.NewErrBadRequestFieldValue("limit", "should be an integer number")
		handleError(err, w, r)
		return
	}
	cols := query.Get("cols")
	request := api.SelectRequest{
		Project:     query.Get("proj"),
		Environment: query.Get("env"),
		Database:    query.Get("db"),
		From:        query.Get("from"),
		SQL:         query.Get("sql"),
		Where:       query.Get("where"),
		Limit:       int(limit),
	}
	if request.Project == "" {
		request.Project = store.SingleProjectID
	}
	if cols != "" {
		request.Columns = strings.Split(cols, ",")
	}
	response, err := api.ExecuteSelect(request)
	returnJSON(w, r, http.StatusOK, err, response)
}
