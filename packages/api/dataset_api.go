package api

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/server/dto"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
	"strings"
)

// GetBoard returns board by ID
func GetRecordsetsSummary(projectID string) (*dto.ProjRecordsetSummary, error) {
	if projectID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("project")
	}
	datasetDefinitions, err := store.Current.LoadRecordsetDefinitions(projectID)
	if err != nil {
		return nil, err
	}
	root := dto.ProjRecordsetSummary{ID: "/"}
	for _, dsDef := range datasetDefinitions {
		dsPath := strings.Split(dsDef.ID, "/")

		var folder *dto.ProjRecordsetSummary
		if len(dsPath) > 1 {
			folder = getRecordsetFolder(&root, dsPath[1:])
		} else {
			folder = &root
		}

		ds := dto.ProjRecordsetSummary{
			ID:    dsDef.ID,
			Title: dsDef.Title,
			Tags:  dsDef.Tags,
		}
		for _, col := range dsDef.Columns {
			ds.Columns = append(ds.Columns, col.Name)
		}

		folder.Recordsets = append(folder.Recordsets, &ds)
	}
	return &root, err
}

func getRecordsetFolder(folder *dto.ProjRecordsetSummary, paths []string) *dto.ProjRecordsetSummary {
	if len(paths) == 0 {
		return folder
	}
	for _, p := range paths {
		for _, rs := range folder.Recordsets {
			if rs.ID == p {
				folder = rs
				continue
			}
		}
		newFolder := &dto.ProjRecordsetSummary{ID: p}
		folder.Recordsets = append(folder.Recordsets, newFolder)
		folder = newFolder
	}
	return folder
}

// GetDatasetDefinition returns definition of a dataset by ID
func GetDatasetDefinition(params RecordsetQueryParams) (dataset *models.RecordsetDefinition, err error) {
	return store.Current.LoadRecordsetDefinition(params.Project, params.Recordset)
}

// SaveBoard saves board
func GetRecordset(params RecordsetDataQueryParams) (recordset *models.Recordset, err error) {
	return store.Current.LoadRecordsetData(params.Project, params.Recordset, params.Data)
}

func AddRecords(projectID, datasetId, recordsetId string, _ []map[string]interface{}) error {
	return errNotImplementedYet
}

type RecordsetQueryParams struct {
	Project   string `json:"project"`
	Recordset string `json:"recordset"`
}

type RecordsetDataQueryParams struct {
	RecordsetQueryParams
	Data string `json:"data"`
}

func (v RecordsetQueryParams) Validate() error {
	if v.Project == "" {
		return validation.NewErrRequestIsMissingRequiredField("project")
	}
	if v.Recordset == "" {
		return validation.NewErrRequestIsMissingRequiredField("recordset")
	}
	return nil
}

func (v RecordsetDataQueryParams) Validate() error {
	if err := v.RecordsetQueryParams.Validate(); err != nil {
		return err
	}
	if v.Data == "" {
		return validation.NewErrRequestIsMissingRequiredField("data")
	}
	return nil
}

type RowValues = map[string]interface{}

type RowWithIndex struct {
	Index  int                    `json:"index"`
	Values map[string]interface{} `json:"values"`
}

func (v RowWithIndex) Validate() error {
	if v.Index < 0 {
		return validation.NewErrBadRecordFieldValue("index", "should be > 0")
	}
	return nil
}

type RowWithIndexAndNewValues struct {
	RowWithIndex
	NewValues map[string]interface{} `json:"newValues"`
}

func AddRowsToRecordset(params RecordsetDataQueryParams, _ []RowValues) (numberOfRecords int, err error) {
	if err = params.Validate(); err != nil {
		return 0, err
	}
	return 0, errNotImplementedYet
}

func RemoveRowsFromRecordset(params RecordsetDataQueryParams, rows []RowWithIndex) (numberOfRecords int, err error) {
	if err = params.Validate(); err != nil {
		return 0, err
	}
	for i, row := range rows {
		if err = row.Validate(); err != nil {
			return 0, fmt.Errorf("invalid row at index=%v: %w", i, err)
		}
	}
	return 0, errNotImplementedYet
}

func UpdateRowsInRecordset(params RecordsetDataQueryParams, rows []RowWithIndexAndNewValues) (numberOfRecords int, err error) {
	if err = params.Validate(); err != nil {
		return 0, err
	}
	for i, row := range rows {
		if err = row.Validate(); err != nil {
			return 0, fmt.Errorf("invalid row at index=%v: %w", i, err)
		}
	}
	return 0, errNotImplementedYet
}
