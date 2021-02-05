package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/validation"
	"os"
	"path"
	"sync"
	"time"
)

func (loader fileSystemLoader) LoadRecordsetDefinitions(projectID string) (datasets []*models.RecordsetDefinition, err error) {
	if projectID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("projectID")
	}
	recordsetsDirPath, err := loader.GetFolderPath(projectID, DataFolder, RecordsetsFolder)
	if err != nil {
		return nil, err
	}
	if err = loadDir(recordsetsDirPath, processDirs, func(files []os.FileInfo) {
		datasets = make([]*models.RecordsetDefinition, 0, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) error {
		datasetID := f.Name()
		dataset, err := loader.LoadRecordsetDefinition(projectID, datasetID)
		if err != nil {
			return err
		}
		if err = dataset.Validate(); err != nil {
			dataset.Errors = append(dataset.Errors, err.Error())
		}
		if mutex != nil {
			mutex.Lock()
		}
		datasets = append(datasets, dataset)
		if mutex != nil {
			mutex.Unlock()
		}
		return nil
	}); err != nil {
		return datasets, err
	}
	return datasets, nil
}

func (loader fileSystemLoader) LoadRecordsetDefinition(projectID, recordsetID string) (dataset *models.RecordsetDefinition, err error) {
	var recordsetsDirPath string
	if recordsetsDirPath, err = loader.GetFolderPath(projectID, DataFolder, RecordsetsFolder); err != nil {
		return
	}
	dataset = new(models.RecordsetDefinition)
	filePath := path.Join(recordsetsDirPath, recordsetID, fmt.Sprintf(".%v.recordset.json", recordsetID))
	if err = readJsonFile(filePath, true, dataset); err != nil {
		err = fmt.Errorf("failed to load dataset [%v] from project [%v]: %w", recordsetID, projectID, err)
		return nil, err
	}
	dataset.ID = recordsetID
	return
}

func (loader fileSystemLoader) LoadRecordsetData(projectID, datasetName, fileName string) (*models.Recordset, error) {
	started := time.Now()
	datasetDef, err := loader.LoadRecordsetDefinition(projectID, datasetName)
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

	recordset.Columns = make([]models.RecordsetColumn, len(datasetDef.Columns))
	for i, field := range datasetDef.Columns {
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
