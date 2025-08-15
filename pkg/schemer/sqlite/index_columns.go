package sqlite

import (
	"context"
	"database/sql"
	// required import
	_ "embed"
	"fmt"
	"github.com/datatug/datatug-core/pkg/models"
	"github.com/datatug/datatug-core/pkg/schemer"
)

var _ schemer.IndexColumnsProvider = (*indexColumnsProvider)(nil)

type indexColumnsProvider struct {
}

func (v indexColumnsProvider) GetIndexColumns(_ context.Context, db *sql.DB, catalog, schema, table, index string) (schemer.IndexColumnsReader, error) {
	if err := verifyTableParams(catalog, schema, table); err != nil {
		return nil, err
	}
	rows, err := db.Query(indexColumnsSQL, index)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve index columns: %w", err)
	}
	return indexColumnsReader{TablePropsReader: schemer.TablePropsReader{
		Table: table,
		Rows:  rows,
	}}, nil
}

var _ schemer.IndexColumnsReader = (*indexColumnsReader)(nil)

type indexColumnsReader struct {
	//index string
	schemer.TablePropsReader
}

//go:embed index_columns.sql
var indexColumnsSQL string

func (s indexColumnsReader) NextIndexColumn() (indexColumn *schemer.IndexColumn, err error) {
	if !s.Rows.Next() {
		err = s.Rows.Err()
		if err != nil {
			err = fmt.Errorf("failed to retrieve index row: %w", err)
		}
		return indexColumn, err
	}
	indexColumn = new(schemer.IndexColumn)
	indexColumn.IndexColumn = new(models.IndexColumn)
	//var objType string
	var cid int
	if err = s.Rows.Scan(
		&cid,
		&indexColumn.Name,
	); err != nil {
		return indexColumn, fmt.Errorf("failed to scan index column row: %w", err)
	}
	return indexColumn, nil
}
