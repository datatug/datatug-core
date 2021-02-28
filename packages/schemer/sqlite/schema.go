package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/schemer"
)

// NewSchemaProvider creates a new SchemaProvider for MS SQL Server
func NewSchemaProvider() schemer.SchemaProvider {
	return schemaProvider{}
}

var _ schemer.SchemaProvider = (*schemaProvider)(nil)

type schemaProvider struct {
	columnsProvider
	constraintsProvider
	indexColumnsProvider
	indexesProvider
	tablesProvider
}

func (schemaProvider) IsBulkProvider() bool {
	return false
}

func (s schemaProvider) RecordsCount(c context.Context, db *sql.DB, catalog, schema, object string) (*int, error) {
	query := fmt.Sprintf("SELECT COUNT(1) FROM [%v].[%v]", schema, object)
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get records count for %v.%v: %w", schema, object, err)
	}
	if rows.Next() {
		var count int
		return &count, rows.Scan(&count)
	}
	return nil, nil
}
