package filestore

import (
	"errors"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/validation"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"
)

// LoadRecordsetDefinitions returns flat list of recordsets that might be stored in a tree structure directories
func (loader fileSystemLoader) LoadRecordsetDefinitions(projectID string) (recordsetDefs []*models.RecordsetDefinition, err error) {
	if projectID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("projectID")
	}
	recordsetsDirPath, err := loader.GetFolderPath(projectID, DataFolder, RecordsetsFolder)
	if err != nil {
		return nil, err
	}
	return loader.loadRecordsetsDir(projectID, "", recordsetsDirPath)
}

func (loader fileSystemLoader) loadRecordsetsDir(projectID, folder, dirPath string) (recordsetDefs []*models.RecordsetDefinition, err error) {
	if err := loadDir(nil, dirPath, processDirs, func(files []os.FileInfo) {
		recordsetDefs = make([]*models.RecordsetDefinition, 0, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) error {
		recordsetID := f.Name() // directory name
		dataset, err := loader.loadRecordsetDefinition(dirPath, folder, recordsetID, projectID)
		if err != nil { // there is no ".<recordsetID>.recordset.json" file in the dir, might be a folder of recordsets
			if errors.Is(err, os.ErrNotExist) {
				subRecordsets, err := loader.loadRecordsetsDir(projectID, path.Join(folder, recordsetID), path.Join(dirPath, recordsetID))
				if err != nil {
					return err
				}
				if len(subRecordsets) > 0 {
					if mutex != nil {
						mutex.Lock()
					}
					recordsetDefs = append(recordsetDefs, subRecordsets...)
					if mutex != nil {
						mutex.Unlock()
					}
				}
				return nil
			}
			return err
		}
		if err = dataset.Validate(); err != nil {
			dataset.Errors = append(dataset.Errors, err.Error())
		}
		if mutex != nil {
			mutex.Lock()
		}
		recordsetDefs = append(recordsetDefs, dataset)
		if mutex != nil {
			mutex.Unlock()
		}
		return nil
	}); err != nil {
		return recordsetDefs, err
	}
	return recordsetDefs, nil
}

// LoadRecordsetDefinition loads recordset definition
func (loader fileSystemLoader) LoadRecordsetDefinition(projectID, recordsetID string) (dataset *models.RecordsetDefinition, err error) {
	var recordsetsDirPath string
	if recordsetsDirPath, err = loader.GetFolderPath(projectID, DataFolder, RecordsetsFolder); err != nil {
		return
	}
	folder := filepath.Dir(recordsetID)
	if len(folder) > 0 {
		recordsetID = recordsetID[len(folder)+1:]
	}
	dirPath := path.Join(recordsetsDirPath, folder)
	return loader.loadRecordsetDefinition(dirPath, folder, recordsetID, projectID)
}

func (loader fileSystemLoader) loadRecordsetDefinition(dirPath, folder, recordsetID, projectID string) (dataset *models.RecordsetDefinition, err error) {
	dataset = new(models.RecordsetDefinition)
	filePath := path.Join(dirPath, recordsetID, fmt.Sprintf(".%v.recordset.json", recordsetID))
	if err = readJsonFile(filePath, true, dataset); err != nil {
		err = fmt.Errorf("failed to load dataset [%v] from project [%v]: %w", recordsetID, projectID, err)
		return nil, err
	}
	if folder == "" {
		dataset.ID = recordsetID
	} else {
		dataset.ID = path.Join(folder, recordsetID)
	}
	return
}

// LoadRecordsetData loads recordset data
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
