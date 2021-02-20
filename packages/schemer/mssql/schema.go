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
	rows, err := s.db.Query(objectsSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query DB objects: %w", err)
	}
	return objectsReader{catalog: catalog, rows: rows}, nil
}

func (s schemaProvider) Columns(_ context.Context, catalog string) (schemer.ColumnsReader, error) {
	rows, err := s.db.Query(columnsSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve columns: %w", err)
	}
	return columnsReader{rows: rows}, nil
}

func (s schemaProvider) Indexes(_ context.Context, catalog string) (schemer.IndexesReader, error) {
	rows, err := s.db.Query(indexesSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve indexes: %w", err)
	}
	return indexesReader{rows: rows}, nil
}

func (s schemaProvider) IndexColumns(_ context.Context, catalog string) (schemer.IndexColumnsReader, error) {
	rows, err := s.db.Query(indexColumnsSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve index columns: %w", err)
	}
	return indexColumnsReader{rows: rows}, nil
}

func (s schemaProvider) Constraints(_ context.Context, catalog string) (schemer.ConstraintsReader, error) {
	rows, err := s.db.Query(constraintsSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve constraints: %w", err)
	}
	return constraintsReader{rows: rows}, nil
}
