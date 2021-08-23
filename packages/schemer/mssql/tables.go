package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/schemer"
)

var _ schemer.TablesProvider = (*tablesProvider)(nil)

type tablesProvider struct {
}

func (v tablesProvider) GetTables(
	_ context.Context,
	db *sql.DB,
	catalog, schema string,
) (
	reader schemer.TablesReader,
	err error,
) {
	if db == nil {
		panic("required parameter db *sql.DB is nil")
	}
	var rows *sql.Rows
	if schema == "" {
		rows, err = db.Query(allTablesSQL)
	} else {
		rows, err = db.Query(tablesForSchemaSQL, schema)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query MSSQL tables: %w", err)
	}
	reader = tablesReader{
		catalog: catalog,
		schema:  schema,
		rows:    rows,
	}
	return reader, nil
}

//goland:noinspection SqlNoDataSourceInspection
const allTablesSQL = `
SELECT
       TABLE_SCHEMA,
       TABLE_NAME,
       TABLE_TYPE
FROM INFORMATION_SCHEMA.TABLES
ORDER BY TABLE_SCHEMA, TABLE_NAME`

const tablesForSchemaSQL = `
SELECT
       TABLE_NAME,
       TABLE_TYPE
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = ?
ORDER BY TABLE_NAME`

var _ schemer.TablesReader = (*tablesReader)(nil)

type tablesReader struct {
	catalog string
	schema  string
	rows    *sql.Rows
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
	var err error
	if s.schema == "" {
		err = s.rows.Scan(&table.Schema, &table.Name, &table.DbType)
	} else {
		err = s.rows.Scan(&table.Name, &table.DbType)
		table.Schema = s.schema
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan table row into Table struct: %w", err)
	}
	table.Catalog = s.catalog
	return &table, nil
}
