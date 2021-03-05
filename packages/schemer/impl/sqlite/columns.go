package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"github.com/datatug/datatug/packages/schemer"
)

var _ schemer.ColumnsProvider = (*columnsProvider)(nil)

type columnsProvider struct {
}

func (v columnsProvider) GetColumns(_ context.Context, db *sql.DB, catalog, schema, table string) (schemer.ColumnsReader, error) {
	if err := verifyTableParams(catalog, schema, table); err != nil {
		return nil, err
	}
	rows, err := db.Query(columnsSQL, table)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve columns for table [%v]: %w", table, err)
	}
	return columnsReader{rows: rows}, nil
}

//go:embed columns.sql
var columnsSQL string

var _ schemer.ColumnsReader = (*columnsReader)(nil)

type columnsReader struct {
	rows *sql.Rows
}

func (s columnsReader) NextColumn() (col schemer.Column, err error) {
	if !s.rows.Next() {
		err = s.rows.Err()
		if err != nil {
			err = fmt.Errorf("failed to retrieve column row: %w", err)
		}
		return col, err
	}
	var cid int
	var isNotNull bool
	var pk int
	if err = s.rows.Scan(
		&cid,
		&col.Name,
		&col.DbType,
		&isNotNull,
		&col.Default,
		&pk,
	); err != nil {
		return col, fmt.Errorf("failed to scan INFORMATION_SCHEMA.COLUMNS row into TableColumn struct: %w", err)
	}
	col.IsNullable = !isNotNull
	return col, nil
}
