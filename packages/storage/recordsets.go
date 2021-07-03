package storage

import "github.com/datatug/datatug/packages/models"

// RecordsetsStore provides access to recordset records
type RecordsetsStore interface {
	ProjectStoreRef
	Recordset(id string) RecordsetLoader
	// LoadRecordsetDefinitions loads list of recordsets summary
	LoadRecordsetDefinitions() (datasets []*models.RecordsetDefinition, err error)
}

// RecordsetLoader loads recordset data
type RecordsetLoader interface {
	// ID returns recordset id
	ID() string
	// LoadRecordsetDefinition loads recordset definition
	LoadRecordsetDefinition() (dataset *models.RecordsetDefinition, err error)
	// LoadRecordsetData loads recordset data
	LoadRecordsetData(ileName string) (recordset *models.Recordset, err error)
}
