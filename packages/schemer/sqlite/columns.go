package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/schemer"
)

var _ schemer.ColumnsProvider = (*columnsProvider)(nil)

type columnsProvider struct {
}

func (v columnsProvider) GetColumns(_ context.Context, db *sql.DB, catalog, schemaName, tableName string) (schemer.ColumnsReader, error) {
	rows, err := db.Query(columnsSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve columns: %w", err)
	}
	return columnsReader{rows: rows}, nil
}

//goland:noinspection SqlNoDataSourceInspection
const columnsSQL = `
SELECT
	cid,
	name,
	type,
  	[notnull],
  	dflt_value,
  	pk
FROM pragma_table_info('%v')
`

var _ schemer.ColumnsReader = (*columnsReader)(nil)

func newColumnsReader(db *sql.DB, schemaName, tableName string) (v columnsReader, err error) {
	if schemaName != "" {
		return v, fmt.Errorf("schema names are not supported by SQLite, got: %v", schemaName)
	}
	v.rows, err = db.Query(fmt.Sprintf(columnsSQL, tableName))
	if err != nil {
		return v, fmt.Errorf("failed to retrieve columns for table [%v]: %w", tableName, err)
	}
	return
}

type columnsReader struct {
	rows *sql.Rows
}

func (s columnsReader) NextColumn() (col schemer.Column, err error) {
	var isNullable string
	var charSetCatalog, charSetSchema, charSetName sql.NullString
	var collationCatalog, collationSchema, collationName sql.NullString
	if !s.rows.Next() {
		err = s.rows.Err()
		if err != nil {
			err = fmt.Errorf("failed to retrieve column row: %w", err)
		}
		return col, err
	}
	if err = s.rows.Scan(
		&col.SchemaName,
		&col.TableName,
		&col.Name,
		&col.OrdinalPosition,
		&col.Default,
		&isNullable,
		&col.DbType,
		&col.CharMaxLength,
		&col.CharOctetLength,
		&charSetCatalog,
		&charSetSchema,
		&charSetName,
		&collationCatalog,
		&collationSchema,
		&collationName,
	); err != nil {
		return col, fmt.Errorf("failed to scan INFORMATION_SCHEMA.COLUMNS row into TableColumn struct: %w", err)
	}
	switch isNullable {
	case "YES":
		col.IsNullable = true
	case "NO":
		col.IsNullable = false
	default:
		err := fmt.Errorf("unknown value for IS_NULLABLE: %v", isNullable)
		return col, err
	}
	if charSetName.Valid && charSetName.String != "" {
		col.CharacterSet = &models.CharacterSet{Name: charSetName.String}
		if charSetSchema.Valid {
			col.CharacterSet.Schema = charSetSchema.String
		}
		if charSetCatalog.Valid {
			col.CharacterSet.Catalog = charSetCatalog.String
		}
	}
	if collationName.Valid && collationName.String != "" {
		col.Collation = &models.Collation{Name: collationName.String}
		//if collationSchema.Valid {
		//	c.Collation.Schema = collationSchema.String
		//}
		//if collationCatalog.Valid {
		//	c.Collation.Catalog = collationCatalog.String
		//}
	}
	/*
		if table == nil || tName != table.ID || tSchema != table.Schema || tCatalog != table.Catalog {
			for _, t := range tables {
				if t.ID == tName && t.Schema == tSchema && t.Catalog == tCatalog {
					//log.Printf("Found table: %+v", t)
					table = t
					break
				}
			}
		}
		if table == nil || table.ID != tName || table.Schema != tSchema || table.Catalog != tCatalog {
		}
	*/
	return col, nil
}
