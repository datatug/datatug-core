package sqlite

import (
	"context"
	"database/sql"
	// required import
	_ "embed"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/schemer"
)

//go:embed objects.sql
var objectsSQL string

var _ schemer.TablesProvider = (*tablesProvider)(nil)

type tablesProvider struct {
}

func (v tablesProvider) GetTables(_ context.Context, db *sql.DB, catalog, schema string) (schemer.TablesReader, error) {
	if err := verifyTableParams(catalog, schema, "tables"); err != nil {
		return nil, err
	}
	rows, err := db.Query(objectsSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query SQLite objects: %w", err)
	}
	return tablesReader{rows: rows}, nil
}

var _ schemer.TablesReader = (*tablesReader)(nil)

type tablesReader struct {
	rows *sql.Rows
}

func (s tablesReader) NextTable() (*models.Table, error) {
	if !s.rows.Next() {
		err := s.rows.Err()
		if err != nil {
			err = fmt.Errorf("failed to retrieve db object row: %w", err)
		}
		return nil, err
	}
	var table models.Table
	if err := s.rows.Scan(&table.DbType, &table.Name, &table.SQL); err != nil {
		return nil, fmt.Errorf("failed to scan table row into Table struct: %w", err)
	}
	table.Schema = "main"
	table.DbType = "BASE TABLE"
	return &table, nil
}
