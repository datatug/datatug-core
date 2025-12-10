package storage

import (
	"context"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// RecordsetsStore provides access to recordset records
type RecordsetsStore interface {
	ProjectStoreRef
	Recordset(id string) RecordsetLoader
	// LoadRecordsetDefinitions loads list of recordsets summary
	LoadRecordsetDefinitions(ctx context.Context) (datasets []*datatug.RecordsetDefinition, err error)
}

// RecordsetLoader loads recordset data
type RecordsetLoader interface {
	// ID returns recordset id
	ID() string
	// LoadRecordsetDefinition loads recordset definition
	LoadRecordsetDefinition(ctx context.Context) (dataset *datatug.RecordsetDefinition, err error)
	// LoadRecordsetData loads recordset data
	LoadRecordsetData(ctx context.Context, fileName string) (recordset *datatug.Recordset, err error)
}
