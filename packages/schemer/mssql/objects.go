package mssql

import (
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/schemer"
)

//goland:noinspection SqlNoDataSourceInspection
const objectsSQL = `
SELECT
       TABLE_SCHEMA,
       TABLE_NAME,
       TABLE_TYPE
FROM INFORMATION_SCHEMA.TABLES
ORDER BY TABLE_SCHEMA, TABLE_NAME`

var _ schemer.ObjectsReader = (*objectsReader)(nil)

type objectsReader struct {
	catalog string
	rows    *sql.Rows
}

func (s objectsReader) NextObject() (*models.Table, error) {
	if !s.rows.Next() {
		err := s.rows.Err()
		if err != nil {
			err = fmt.Errorf("failed to retrieve db object row: %w", err)
		}
		return nil, err
	}
	var table models.Table
	if err := s.rows.Scan(&table.Schema, &table.Name, &table.DbType); err != nil {
		return nil, fmt.Errorf("failed to scan table row into Table struct: %w", err)
	}
	table.Catalog = s.catalog
	return &table, nil
}
