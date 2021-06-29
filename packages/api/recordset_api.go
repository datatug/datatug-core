package api

import (
	"fmt"
	dto2 "github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"github.com/strongo/validation"
	"strings"
)

// GetRecordsetsSummary returns board by ID
func GetRecordsetsSummary(ref dto2.ProjectRef) (*dto2.ProjRecordsetSummary, error) {
	if ref.ProjectID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("project")
	}
	dal, err := storage.NewDatatugStore(ref.StoreID)
	if err != nil {
		return nil, err
	}
	datasetDefinitions, err := dal.LoadRecordsetDefinitions(ref.ProjectID)
	if err != nil {
		return nil, err
	}
	root := dto2.ProjRecordsetSummary{}
	root.ID = "/"
	for _, dsDef := range datasetDefinitions {
		dsPath := strings.Split(dsDef.ID, "/")

		var folder *dto2.ProjRecordsetSummary
		if len(dsPath) > 1 {
			folder = getRecordsetFolder(&root, dsPath[1:])
		} else {
			folder = &root
		}

		ds := dto2.ProjRecordsetSummary{
			ProjectItem: models.ProjectItem{
				ID:    dsDef.ID,
				Title: dsDef.Title,
				ListOfTags: models.ListOfTags{
					Tags: dsDef.Tags,
				},
			},
		}
		for _, col := range dsDef.Columns {
			ds.Columns = append(ds.Columns, col.Name)
		}

		folder.Recordsets = append(folder.Recordsets, &ds)
	}
	return &root, err
}

func getRecordsetFolder(folder *dto2.ProjRecordsetSummary, paths []string) *dto2.ProjRecordsetSummary {
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
		newFolder := &dto2.ProjRecordsetSummary{
			ProjectItem: models.ProjectItem{ID: p},
		}
		folder.Recordsets = append(folder.Recordsets, newFolder)
		folder = newFolder
	}
	return folder
}

// GetDatasetDefinition returns definition of a dataset by ID
func GetDatasetDefinition(ref dto2.ProjectItemRef) (dataset *models.RecordsetDefinition, err error) {
	dal, err := storage.NewDatatugStore(ref.StoreID)
	if err != nil {
		return nil, err
	}
	return dal.LoadRecordsetDefinition(ref.ProjectID, ref.ID)
}

// GetRecordset saves board
func GetRecordset(ref dto2.ProjectItemRef) (recordset *models.Recordset, err error) {
	dal, err := storage.NewDatatugStore(ref.StoreID)
	if err != nil {
		return nil, err
	}
	return dal.LoadRecordsetData(ref.ProjectID, ref.ID, "")
}

// AddRecords adds record
func AddRecords(projectID, datasetID, recordsetID string, _ []map[string]interface{}) error {
	return errNotImplementedYet
}

// RecordsetRequestParams is a set of common request parameters
type RecordsetRequestParams struct {
	Project   string `json:"project"`
	Recordset string `json:"recordset"`
}

// RecordsetDataRequestParams is a set of common request parameters
type RecordsetDataRequestParams struct {
	RecordsetRequestParams
	Data string `json:"data"`
}

// Validate returns error if not valid
func (v RecordsetRequestParams) Validate() error {
	if v.Project == "" {
		return validation.NewErrRequestIsMissingRequiredField("project")
	}
	if v.Recordset == "" {
		return validation.NewErrRequestIsMissingRequiredField("recordset")
	}
	return nil
}

// Validate returns error if not valid
func (v RecordsetDataRequestParams) Validate() error {
	if err := v.RecordsetRequestParams.Validate(); err != nil {
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
func AddRowsToRecordset(params RecordsetDataRequestParams, _ []RowValues) (numberOfRecords int, err error) {
	if err = params.Validate(); err != nil {
		return 0, err
	}
	return 0, errNotImplementedYet
}

// RemoveRowsFromRecordset removes rows from a recordset
func RemoveRowsFromRecordset(params RecordsetDataRequestParams, rows []RowWithIndex) (numberOfRecords int, err error) {
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
func UpdateRowsInRecordset(params RecordsetDataRequestParams, rows []RowWithIndexAndNewValues) (numberOfRecords int, err error) {
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
