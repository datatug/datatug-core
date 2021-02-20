package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/schemer"
)

// NewSchemaProvider creates a new SchemaProvider for MS SQL Server
func NewSchemaProvider(db *sql.DB) schemer.SchemaProvider {
	return schemaProvider{db: db}
}

var _ schemer.SchemaProvider = (*schemaProvider)(nil)

type schemaProvider struct {
	db *sql.DB
}

func (s schemaProvider) Objects(_ context.Context, catalog string) (schemer.ObjectsReader, error) {
	//goland:noinspection SqlNoDataSourceInspection
	rows, err := s.db.Query(`
SELECT
       TABLE_SCHEMA,
       TABLE_NAME,
       TABLE_TYPE
FROM INFORMATION_SCHEMA.TABLES
ORDER BY TABLE_SCHEMA, TABLE_NAME`)
	if err != nil {
		return nil, fmt.Errorf("failed to query DB objects: %w", err)
	}
	return objectsReader{catalog: catalog, rows: rows}, nil
}

func (s schemaProvider) Columns(_ context.Context, catalog string) (schemer.ColumnsReader, error) {
	//goland:noinspection SqlNoDataSourceInspection
	rows, err := s.db.Query(`
SELECT
    TABLE_SCHEMA,
    TABLE_NAME,
    COLUMN_NAME,
    ORDINAL_POSITION,
    COLUMN_DEFAULT,
    IS_NULLABLE,
    DATA_TYPE,
    CHARACTER_MAXIMUM_LENGTH,
    CHARACTER_OCTET_LENGTH,
	CHARACTER_SET_CATALOG,
	CHARACTER_SET_SCHEMA,
    CHARACTER_SET_NAME,
	COLLATION_CATALOG,
	COLLATION_SCHEMA,
    COLLATION_NAME
FROM INFORMATION_SCHEMA.COLUMNS ORDER BY TABLE_SCHEMA, TABLE_NAME, ORDINAL_POSITION`)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %w", err)
	}
	return columnsReader{rows: rows}, nil
}

func (s schemaProvider) Indexes(_ context.Context, catalog string) (schemer.IndexesReader, error) {
	//goland:noinspection SqlNoDataSourceInspection
	rows, err := s.db.Query(indexesSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query indexes: %w", err)
	}
	return indexesReader{rows: rows}, nil
}

func (s schemaProvider) IndexColumns(_ context.Context, catalog string) (schemer.IndexColumnsReader, error) {
	//goland:noinspection SqlNoDataSourceInspection
	rows, err := s.db.Query(indexColumnsSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query indexes: %w", err)
	}
	return indexColumnsReader{rows: rows}, nil
}

func (s schemaProvider) Constraints(_ context.Context, catalog string) (schemer.ConstraintsReader, error) {
	//goland:noinspection SqlNoDataSourceInspection
	rows, err := s.db.Query(constraintsSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query constraints: %w", err)
	}
	return constraintsReader{rows: rows}, nil
}
