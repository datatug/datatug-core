package endpoints

import (
	"encoding/json"
	"fmt"
	"github.com/datatug/datatug/packages/api"
	"github.com/strongo/validation"
	"log"
	"net/http"
	"strconv"
)

func getRecordsetRequestParams(r *http.Request) (params api.RecordsetRequestParams, err error) {
	query := r.URL.Query()
	if params.Project = query.Get(urlQueryParamProjectID); params.Project == "" {
		err = validation.NewErrRequestIsMissingRequiredField(urlQueryParamProjectID)
		return
	}
	if params.Recordset = query.Get(urlQueryParamRecordsetID); params.Recordset == "" {
		err = validation.NewErrRequestIsMissingRequiredField(urlQueryParamRecordsetID)
		return
	}
	return
}

func getRecordsetDataParams(r *http.Request) (params api.RecordsetDataRequestParams, err error) {
	query := r.URL.Query()
	params.RecordsetRequestParams, err = getRecordsetRequestParams(r)

	if params.Data = query.Get(urlQueryParamDataID); params.Data == "" {
		err = validation.NewErrRequestIsMissingRequiredField(urlQueryParamDataID)
		return
	}
	return
}

// GetRecordsetsSummary returns list of dataset definitions
func GetRecordsetsSummary(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	ref := newProjectRef(query)
	ctx, err := GetContext(r.Context())
	if err != nil {
		handleError(err, w, r)
	}
	datasets, err := api.GetRecordsetsSummary(ctx, ref)
	returnJSON(w, r, http.StatusOK, err, datasets)
}

// GetRecordsetDefinition returns list of dataset definitions
func GetRecordsetDefinition(w http.ResponseWriter, r *http.Request) {
	ctx, err := GetContext(r.Context())
	if err != nil {
		handleError(err, w, r)
	}
	ref := newProjectItemRef(r.URL.Query())
	datasets, err := api.GetDatasetDefinition(ctx, ref)
	returnJSON(w, r, http.StatusOK, err, datasets)
}

// GetRecordsetData returns data
func GetRecordsetData(w http.ResponseWriter, r *http.Request) {
	ctx, err := GetContext(r.Context())
	if err != nil {
		handleError(err, w, r)
	}
	ref := newProjectItemRef(r.URL.Query())
	recordset, err := api.GetRecordset(ctx, ref)
	returnJSON(w, r, http.StatusOK, err, recordset)
}

// AddRowsToRecordset adds rows to a recordset
func AddRowsToRecordset(w http.ResponseWriter, r *http.Request) {
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

// DeleteRowsFromRecordset deletes rows from a recordset
func DeleteRowsFromRecordset(w http.ResponseWriter, r *http.Request) {
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

// UpdateRowsInRecordset updates rows in a recordset
func UpdateRowsInRecordset(w http.ResponseWriter, r *http.Request) {
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
