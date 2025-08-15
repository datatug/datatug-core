package endpoints

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/datatug/datatug-core/pkg/api"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/sneat-co/sneat-go-core/apicore"
	"github.com/strongo/validation"
	"log"
	"net/http"
	"strconv"
)

func getRecordsetRequestParams(r *http.Request) (params api.RecordsetRequestParams, err error) {
	query := r.URL.Query()
	if params.Project = query.Get(urlParamProjectID); params.Project == "" {
		err = validation.NewErrRequestIsMissingRequiredField(urlParamProjectID)
		return
	}
	if params.Recordset = query.Get(urlParamRecordsetID); params.Recordset == "" {
		err = validation.NewErrRequestIsMissingRequiredField(urlParamRecordsetID)
		return
	}
	return
}

func getRecordsetDataParams(r *http.Request) (params api.RecordsetDataRequestParams, err error) {
	query := r.URL.Query()
	params.RecordsetRequestParams, err = getRecordsetRequestParams(r)

	if params.Data = query.Get(urlParamDataID); params.Data == "" {
		err = validation.NewErrRequestIsMissingRequiredField(urlParamDataID)
		return
	}
	return
}

// getRecordsetsSummary returns list of dataset definitions
func getRecordsetsSummary(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	ref := newProjectRef(query)
	ctx, err := getContextFromRequest(r)
	if err != nil {
		handleError(err, w, r)
	}
	datasets, err := api.GetRecordsetsSummary(ctx, ref)
	returnJSON(w, r, http.StatusOK, err, datasets)
}

// getRecordsetDefinition returns list of dataset definitions
func getRecordsetDefinition(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectItemRef
	getProjectItem(w, r, &ref, func(ctx context.Context) (responseDTO apicore.ResponseDTO, err error) {
		return api.GetDatasetDefinition(ctx, ref)
	})
}

// getRecordsetData returns data
func getRecordsetData(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectItemRef
	getProjectItem(w, r, &ref, func(ctx context.Context) (responseDTO apicore.ResponseDTO, err error) {
		return api.GetRecordset(ctx, ref)
	})
}

// addRowsToRecordset adds rows to a recordset
func addRowsToRecordset(w http.ResponseWriter, r *http.Request) {
	var err error
	params, err := getRecordsetDataParams(r)
	if err != nil {
		handleError(err, w, r)
		return
	}

	count, err := strconv.Atoi(r.URL.Query().Get("count"))
	if err != nil {
		log.Println(fmt.Errorf("WARNING: count parameter is not supplied or invalid: %w", err))
	}
	rows := make([]api.RowValues, 0, count)

	decoder := json.NewDecoder(r.Body)
	if err = decoder.Decode(&rows); err != nil {
		err = validation.NewErrBadRequestFieldValue("body", err.Error())
		handleError(err, w, r)
		return
	}
	numberOfRecords, err := api.AddRowsToRecordset(params, nil)
	returnJSON(w, r, http.StatusCreated, err, numberOfRecords)
}

// deleteRowsFromRecordset deletes rows from a recordset
func deleteRowsFromRecordset(w http.ResponseWriter, r *http.Request) {
	params, err := getRecordsetDataParams(r)
	if err != nil {
		handleError(err, w, r)
		return
	}
	count, err := strconv.Atoi(r.URL.Query().Get("count"))
	if err != nil {
		log.Println(fmt.Errorf("WARNING: count parameter is not supplied or invalid: %w", err))
	}
	rows := make([]api.RowWithIndex, 0, count)
	numberOfRecords, err := api.RemoveRowsFromRecordset(params, rows)
	returnJSON(w, r, http.StatusCreated, err, numberOfRecords)
}

// updateRowsInRecordset updates rows in a recordset
func updateRowsInRecordset(w http.ResponseWriter, r *http.Request) {
	params, err := getRecordsetDataParams(r)
	if err != nil {
		handleError(err, w, r)
		return
	}
	count, err := strconv.Atoi(r.URL.Query().Get("count"))
	if err != nil {
		log.Println(fmt.Errorf("WARNING: count parameter is not supplied or invalid: %w", err))
	}
	rows := make([]api.RowWithIndexAndNewValues, 0, count)
	numberOfRecords, err := api.UpdateRowsInRecordset(params, rows)
	returnJSON(w, r, http.StatusCreated, err, numberOfRecords)
}
