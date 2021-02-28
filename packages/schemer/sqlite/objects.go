package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/schemer"
)

var _ schemer.ObjectsProvider = (*objectsProvider)(nil)

type objectsProvider struct {
}

func (v objectsProvider) Objects(_ context.Context, db *sql.DB, catalog, schema string) (schemer.ObjectsReader, error) {
	rows, err := db.Query(objectsSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query DB objects: %w", err)
	}
	return objectsReader{catalog: catalog, rows: rows}, nil
}

//goland:noinspection SqlNoDataSourceInspection
const objectsSQL = `
SELECT
    m.type,
    --m.name
    m.tbl_name,
	m.sql
FROM sqlite_master m
WHERE m.type in ('table', 'view')
ORDER BY m.name`

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
	if err := s.rows.Scan(&table.DbType, &table.Name, &table.SQL); err != nil {
		return nil, fmt.Errorf("failed to scan table row into Table struct: %w", err)
	}
	table.Catalog = s.catalog
	return &table, nil
}
