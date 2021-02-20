package mssql

import (
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/schemer"
)

var _ schemer.ColumnsReader = (*columnsReader)(nil)

type columnsReader struct {
	rows *sql.Rows
}

func (s columnsReader) NextColumn() (col schemer.Column, err error) {
	var isNullable string
	var charSetCatalog, charSetSchema, charSetName sql.NullString
	var collationCatalog, collationSchema, collationName sql.NullString
	if !s.rows.Next() {
		return col, fmt.Errorf("failed to retrieve column row: %w", s.rows.Err())
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
