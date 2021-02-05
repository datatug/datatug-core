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

func getRecordsetParams(r *http.Request) (params api.RecordsetQueryParams, err error) {
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

func getRecordsetDataParams(r *http.Request) (params api.RecordsetDataQueryParams, err error) {
	query := r.URL.Query()
	params.RecordsetQueryParams, err = getRecordsetParams(r)

	if params.Data = query.Get(urlQueryParamDataID); params.Data == "" {
		err = validation.NewErrRequestIsMissingRequiredField(urlQueryParamDataID)
		return
	}
	return
}

// GetRecordsetsSummary returns list of dataset definitions
func GetRecordsetsSummary(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	projectID := query.Get(urlQueryParamProjectID)
	if projectID == "" {
		handleError(validation.NewErrRequestIsMissingRequiredField(urlQueryParamProjectID), w, r)
		return
	}
	datasets, err := api.GetRecordsetsSummary(projectID)
	ReturnJSON(w, r, http.StatusOK, err, datasets)
}

// GetRecordsetDefinition returns list of dataset definitions
func GetRecordsetDefinition(w http.ResponseWriter, r *http.Request) {
	params, err := getRecordsetParams(r)
	if handleError(err, w, r) {
		return
	}
	datasets, err := api.GetDatasetDefinition(params)
	ReturnJSON(w, r, http.StatusOK, err, datasets)
}

// GetRecordsetData returns data
func GetRecordsetData(w http.ResponseWriter, r *http.Request) {
	params, err := getRecordsetDataParams(r)
	if handleError(err, w, r) {
		return
	}
	recordset, err := api.GetRecordset(params)
	ReturnJSON(w, r, http.StatusOK, err, recordset)
}

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
	ReturnJSON(w, r, http.StatusCreated, err, numberOfRecords)
}

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
	ReturnJSON(w, r, http.StatusCreated, err, numberOfRecords)
}

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
	ReturnJSON(w, r, http.StatusCreated, err, numberOfRecords)
}
