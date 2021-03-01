package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/schemer"
)

var _ schemer.IndexColumnsProvider = (*indexColumnsProvider)(nil)

type indexColumnsProvider struct {
}

func (v indexColumnsProvider) GetIndexColumns(_ context.Context, db *sql.DB, catalog, schema, table, index string) (schemer.IndexColumnsReader, error) {
	if err := verifyTableParams(catalog, schema, table); err != nil {
		return nil, err
	}
	rows, err := db.Query(indexColumnsSQL)
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
	index string
	schemer.TablePropsReader
}

//goland:noinspection SqlNoDataSourceInspection
const indexColumnsSQL = `
SELECT
	cid,
	name
FROM PRAGMA_index_info('IFK_AlbumArtistId')
ORDER BY seqno
`

func (s indexColumnsReader) NextIndexColumn() (indexColumn *schemer.IndexColumn, err error) {
	if !s.Rows.Next() {
		err = s.Rows.Err()
		if err != nil {
			err = fmt.Errorf("failed to retrieve index row: %w", err)
		}
		return indexColumn, err
	}
	indexColumn.IndexColumn = new(models.IndexColumn)
	//var objType string
	var keyOrdinal, partitionOrdinal, columnStoreOrderOrdinal int
	if err = s.Rows.Scan(
		&indexColumn.TableName,
		//&objType,
		//&indexColumn.TableType,
		&indexColumn.IndexName,
		&indexColumn.Name,
		&keyOrdinal,
		&partitionOrdinal,
		&indexColumn.IsDescending,
		&indexColumn.IsIncludedColumn,
		&columnStoreOrderOrdinal,
	); err != nil {
		return indexColumn, fmt.Errorf("failed to scan index column row: %w", err)
	}
	return indexColumn, nil
}
