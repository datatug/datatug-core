package endpoints

import (
	"encoding/json"
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
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Only POST requests are supported"))
	}

	response, err := api.ExecuteCommands(executeRequest)
	ReturnJSON(w, r, http.StatusOK, err, response)
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
	ReturnJSON(w, r, http.StatusOK, err, response)
}
