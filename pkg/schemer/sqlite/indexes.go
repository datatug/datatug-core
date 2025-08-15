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

var _ schemer.IndexesProvider = (*indexesProvider)(nil)

type indexesProvider struct {
}

func (v indexesProvider) GetIndexes(_ context.Context, db *sql.DB, catalog, schema, table string) (schemer.IndexesReader, error) {
	if err := verifyTableParams(catalog, schema, table); err != nil {
		return nil, err
	}
	rows, err := db.Query(indexesSQL, table)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve indexes: %w", err)
	}
	return indexesReader{schemer.TablePropsReader{Table: table, Rows: rows}}, nil
}

var _ = (schemer.IndexesReader)(nil)

type indexesReader struct {
	schemer.TablePropsReader
}

/*
case when i.type = 1 then 'Clustered index'
when i.type = 2 then 'Nonclustered unique index'
when i.type = 3 then 'XML index'
when i.type = 4 then 'Spatial index'
when i.type = 5 then 'Clustered columnstore index'
when i.type = 6 then 'Nonclustered columnstore index'
when i.type = 7 then 'Nonclustered hash index'
end as type_description,
*/

//go:embed indexes.sql
var indexesSQL string

func (s indexesReader) NextIndex() (index *schemer.Index, err error) {
	if !s.Rows.Next() {
		err = s.Rows.Err()
		if err != nil {
			err = fmt.Errorf("failed to retrieve index row: %w", s.Rows.Err())
		}
		return index, err
	}
	index = new(schemer.Index)
	index.Index = new(models.Index)
	if err = s.Rows.Scan(
		&index.Name,
		&index.IsUnique,
		&index.Origin,
		&index.IsPartial,
	); err != nil {
		return index, fmt.Errorf("failed to scan index row: %w", err)
	}
	index.TableName = s.Table
	switch index.Origin {
	case "pk":
		index.IsPrimaryKey = true
	case "c":
		//
	}
	return index, nil
}
