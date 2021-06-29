package storage

import "github.com/datatug/datatug/packages/models"

type RecordsetsStore interface {
	Loader() RecordsetsLoader
}

// RecordsetsLoader loads recordsets
type RecordsetsLoader interface {
	// LoadRecordsetDefinitions loads list of recordsets summary
	LoadRecordsetDefinitions() (datasets []*models.RecordsetDefinition, err error)
	// LoadRecordsetDefinition loads recordset definition
	LoadRecordsetDefinition(datasetName string) (dataset *models.RecordsetDefinition, err error)
	// LoadRecordsetData loads recordset data
	LoadRecordsetData(datasetName, fileName string) (recordset *models.Recordset, err error)
}
