package endpoints

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/execute"
	"github.com/datatug/datatug/packages/models"
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
	for qpName, qpValue := range query {
		if !strings.HasPrefix(qpName, "p:") || len(qpName) == 2 {
			continue
		}
		pKey := qpName[2:]
		i := strings.Index(pKey, ":")
		pType := pKey[i+1:]
		var pVal interface{}
		switch pType {
		case "integer":
			pVal, err = strconv.Atoi(qpValue[0])
			if err != nil {
				handleError(validation.NewErrBadRequestFieldValue(qpName, fmt.Sprintf("not an integer: %v", err)), w, r)
				return
			}
		case "number":
		case "string":
		case "boolean":
			switch strings.ToLower(qpValue[0]) {
			case "true", "1", "yes":
				pVal = true
			case "", "false", "no":
				pVal = false
			}
		// OK
		default:
			handleError(validation.NewErrBadRecordFieldValue(qpName, "unknown or unsupported parameter type: "+pType), w, r)
			return
		}
		request.Parameters = append(request.Parameters, models.Parameter{
			ID:    pKey,
			Type:  pType,
			Value: pVal,
		})
		qpName = qpName[:i]

	}
	if cols != "" {
		request.Columns = strings.Split(cols, ",")
	}
	response, err := api.ExecuteSelect(request)
	returnJSON(w, r, http.StatusOK, err, response)
}
