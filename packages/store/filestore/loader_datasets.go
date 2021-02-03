package filestore

import (
	"errors"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"path"
	"time"
)

func (fileSystemLoader) LoadDatasets(projectID string) (datasets []models.DatasetDefinition, err error) {
	return nil, errors.New("not implemented yet")
}

func (loader fileSystemLoader) LoadDatasetDefinition(projectID, datasetName string) (dataset *models.DatasetDefinition, err error) {
	var projPath string
	if projectID, projPath, err = loader.GetProjectPath(projectID); err != nil {
		return
	}
	filePath := path.Join(projPath, DatatugFolder, DataFolder, datasetName, fmt.Sprintf(".%v.datatug.json", datasetName))
	dataset = new(models.DatasetDefinition)
	if err = readJsonFile(filePath, true, dataset); err != nil {
		err = fmt.Errorf("failed to load dataset [%v] from project [%v]: %w", datasetName, projectID, err)
		return nil, err
	}
	return
}

func (loader fileSystemLoader) LoadRecordset(projectID, datasetName, fileName string) (*models.Recordset, error) {
	started := time.Now()
	datasetDef, err := loader.LoadDatasetDefinition(projectID, datasetName)
	if err != nil {
		return nil, err
	}

	var projPath string
	if _, projPath, err = loader.GetProjectPath(projectID); err != nil {
		return nil, err
	}
	filePath := path.Join(projPath, DatatugFolder, DataFolder, datasetName, fileName)
	var recordset models.Recordset
	rows := make([]interface{}, 0)
	if err := readJsonFile(filePath, true, &rows); err != nil {
		return nil, err
	}

	recordset.Columns = make([]models.RecordsetColumn, len(datasetDef.Fields))
	for i, field := range datasetDef.Fields {
		recordset.Columns[i] = models.RecordsetColumn{
			Name:   field.Name,
			DbType: field.Type,
			Meta:   field.Meta,
		}
	}

	recordset.Rows = make([][]interface{}, 0, len(rows))
	for i, row := range rows {
		valuesByName, ok := row.(map[string]interface{})
		if !ok {
			return &recordset, fmt.Errorf("unexpected row type at index=%v: %T", i, row)
		}
		values := make([]interface{}, len(recordset.Columns))
		for i, col := range recordset.Columns {
			if value, ok := valuesByName[col.Name]; ok {
				values[i] = value
			} else {
				_, _ = fmt.Printf("\t%v: %+v\n", col.Name, nil)
			}
		}
		recordset.Rows = append(recordset.Rows, values)
	}

	recordset.Duration = time.Since(started)
	return &recordset, nil
}
