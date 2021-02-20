package mssql

import (
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/schemer"
)

var _ schemer.IndexColumnsReader = (*indexColumnsReader)(nil)

type indexColumnsReader struct {
	rows *sql.Rows
}

//goland:noinspection SqlNoDataSourceInspection
const indexColumnsSQL = `
SELECT
	SCHEMA_NAME(o.schema_id) AS schema_name,
	o.name AS object_name,
	--o.type AS object_type,
	--o.type_desc AS object_type_desc,
	i.name AS index_name,
	c.name AS column_name,
	ic.key_ordinal,
	ic.partition_ordinal,
	ic.is_descending_key,
	ic.is_included_column,
	ic.column_store_order_ordinal
FROM sys.index_columns AS ic
INNER JOIN sys.columns AS c ON c.object_id = ic.object_id AND c.column_id = ic.column_id
INNER JOIN sys.indexes AS i ON i.object_id = ic.object_id AND i.index_id = ic.index_id
INNER JOIN sys.objects o ON o.object_id = ic.object_id
WHERE o.is_ms_shipped <> 1 --and i.index_id > 0
ORDER BY SCHEMA_NAME(o.schema_id), o.name, i.name, ic.key_ordinal
`

func (s indexColumnsReader) NextIndexColumn() (indexColumn schemer.IndexColumn, err error) {
	if !s.rows.Next() {
		err = s.rows.Err()
		if err != nil {
			err = fmt.Errorf("failed to retrieve index row: %w", err)
		}
		return indexColumn, err
	}
	indexColumn.IndexColumn = new(models.IndexColumn)
	//var objType string
	var keyOrdinal, partitionOrdinal, columnStoreOrderOrdinal int
	if err = s.rows.Scan(
		&indexColumn.SchemaName,
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
