package api

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/server/dto"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
	"strings"
)

// GetRecordsetsSummary returns board by ID
func GetRecordsetsSummary(projectID string) (*dto.ProjRecordsetSummary, error) {
	if projectID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("project")
	}
	datasetDefinitions, err := store.Current.LoadRecordsetDefinitions(projectID)
	if err != nil {
		return nil, err
	}
	root := dto.ProjRecordsetSummary{}
	root.ID = "/"
	for _, dsDef := range datasetDefinitions {
		dsPath := strings.Split(dsDef.ID, "/")

		var folder *dto.ProjRecordsetSummary
		if len(dsPath) > 1 {
			folder = getRecordsetFolder(&root, dsPath[1:])
		} else {
			folder = &root
		}

		ds := dto.ProjRecordsetSummary{
			ProjectItem: models.ProjectItem{
				ID:    dsDef.ID,
				Title: dsDef.Title,
				Tags:  dsDef.Tags,
			},
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
		newFolder := &dto.ProjRecordsetSummary{
			ProjectItem: models.ProjectItem{ID: p},
		}
		folder.Recordsets = append(folder.Recordsets, newFolder)
		folder = newFolder
	}
	return folder
}

// GetDatasetDefinition returns definition of a dataset by ID
func GetDatasetDefinition(params RecordsetQueryParams) (dataset *models.RecordsetDefinition, err error) {
	return store.Current.LoadRecordsetDefinition(params.Project, params.Recordset)
}

// GetRecordset saves board
func GetRecordset(params RecordsetDataQueryParams) (recordset *models.Recordset, err error) {
	return store.Current.LoadRecordsetData(params.Project, params.Recordset, params.Data)
}

// AddRecords adds record
func AddRecords(projectID, datasetID, recordsetID string, _ []map[string]interface{}) error {
	return errNotImplementedYet
}

// RecordsetQueryParams is a set of common request parameters
type RecordsetQueryParams struct {
	Project   string `json:"project"`
	Recordset string `json:"recordset"`
}

// RecordsetDataQueryParams is a set of common request parameters
type RecordsetDataQueryParams struct {
	RecordsetQueryParams
	Data string `json:"data"`
}

// Validate returns error if not valid
func (v RecordsetQueryParams) Validate() error {
	if v.Project == "" {
		return validation.NewErrRequestIsMissingRequiredField("project")
	}
	if v.Recordset == "" {
		return validation.NewErrRequestIsMissingRequiredField("recordset")
	}
	return nil
}

// Validate returns error if not valid
func (v RecordsetDataQueryParams) Validate() error {
	if err := v.RecordsetQueryParams.Validate(); err != nil {
		return err
	}
	if v.Data == "" {
		return validation.NewErrRequestIsMissingRequiredField("data")
	}
	return nil
}

// RowValues set of named values
type RowValues = map[string]interface{}

// RowWithIndex points to specific row with expected values
type RowWithIndex struct {
	Index  int                    `json:"index"`
	Values map[string]interface{} `json:"values"`
}

// Validate returns error if not valid
func (v RowWithIndex) Validate() error {
	if v.Index < 0 {
		return validation.NewErrBadRecordFieldValue("index", "should be > 0")
	}
	return nil
}

// RowWithIndexAndNewValues points to specific row with expected values and provides new set of named values
type RowWithIndexAndNewValues struct {
	RowWithIndex
	NewValues map[string]interface{} `json:"newValues"`
}

// AddRowsToRecordset adds rows to a recordset
func AddRowsToRecordset(params RecordsetDataQueryParams, _ []RowValues) (numberOfRecords int, err error) {
	if err = params.Validate(); err != nil {
		return 0, err
	}
	return 0, errNotImplementedYet
}

// RemoveRowsFromRecordset removes rows from a recordset
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

// UpdateRowsInRecordset updates rows in a recordset
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
