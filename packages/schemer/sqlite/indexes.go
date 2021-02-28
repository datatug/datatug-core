package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/schemer"
)

var _ schemer.IndexesProvider = (*indexesProvider)(nil)

type indexesProvider struct {
}

func (v indexesProvider) Indexes(_ context.Context, db *sql.DB, catalog, schema, table string) (schemer.IndexesReader, error) {
	rows, err := db.Query(indexesSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve indexes: %w", err)
	}
	return indexesReader{rows: rows}, nil
}

var _ = (schemer.IndexesReader)(nil)

type indexesReader struct {
	rows *sql.Rows
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

//goland:noinspection SqlNoDataSourceInspection
const indexesSQL = `
SELECT 
    SCHEMA_NAME(o.schema_id) AS schema_name,
	o.name AS object_name,
    CASE WHEN o.type = 'U' THEN 'Table' WHEN o.type = 'V' THEN 'View' ELSE o.type END AS object_type,
    i.name,
	i.type,
	i.type_desc,
	is_unique,
	is_primary_key,
	is_unique_constraint
FROM sys.indexes AS i
INNER JOIN sys.objects o ON o.object_id = i.object_id
WHERE o.is_ms_shipped <> 1 AND i.type > 0
ORDER BY SCHEMA_NAME(o.schema_id) + '.' + o.name, i.name
`

func (s indexesReader) NextIndex() (index schemer.Index, err error) {
	if !s.rows.Next() {
		err = s.rows.Err()
		if err != nil {
			err = fmt.Errorf("failed to retrieve index row: %w", s.rows.Err())
		}
		return index, err
	}
	index.Index = new(models.Index)
	var iType int
	if err = s.rows.Scan(
		&index.SchemaName,
		&index.TableName,
		&index.TableType,
		&index.Name,
		&iType,
		&index.Type,
		&index.IsUnique,
		&index.IsPrimaryKey,
		&index.IsUniqueConstraint,
	); err != nil {
		return index, fmt.Errorf("failed to scan index row: %w", err)
	}
	switch iType {
	case 1:
		index.IsClustered = true
	case 3:
		index.IsXml = true
	case 5:
		index.IsClustered = true
		index.IsColumnStore = true
	case 6:
		index.IsColumnStore = true
	case 7:
		index.IsHash = true
	}
	return index, nil
}
