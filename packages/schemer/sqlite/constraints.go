package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/schemer"
)

var _ schemer.ConstraintsProvider = (*constraintsProvider)(nil)

type constraintsProvider struct {
}

func (v constraintsProvider) GetConstraints(c context.Context, db *sql.DB, catalog, schema, table string) (schemer.ConstraintsReader, error) {
	rows, err := db.Query(constraintsSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve constraints: %w", err)
	}
	return constraintsReader{rows: rows}, nil
}

var _ schemer.ConstraintsReader = (*constraintsReader)(nil)

//goland:noinspection SqlNoDataSourceInspection
const constraintsSQL = `
SELECT
	tc.TABLE_SCHEMA, tc.TABLE_NAME,
    tc.CONSTRAINT_TYPE, kcu.CONSTRAINT_NAME,
    kcu.COLUMN_NAME,-- kcu.ORDINAL_POSITION,
	rc.UNIQUE_CONSTRAINT_CATALOG, rc.UNIQUE_CONSTRAINT_SCHEMA, rc.UNIQUE_CONSTRAINT_NAME,
    rc.MATCH_OPTION, rc.UPDATE_RULE, rc.DELETE_RULE,
	kcu2.TABLE_CATALOG AS REF_TABLE_CATALOG, kcu2.TABLE_SCHEMA AS REF_TABLE_SCHEMA, kcu2.TABLE_NAME AS REF_TABLE_NAME, kcu2.COLUMN_NAME AS REF_COL_NAME
FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE AS kcu
INNER JOIN INFORMATION_SCHEMA.TABLE_CONSTRAINTS AS tc ON tc.CONSTRAINT_CATALOG = kcu.CONSTRAINT_CATALOG AND tc.CONSTRAINT_SCHEMA = kcu.CONSTRAINT_SCHEMA AND tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
LEFT JOIN INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS AS rc ON tc.CONSTRAINT_TYPE = 'FOREIGN KEY' AND rc.CONSTRAINT_CATALOG = tc.CONSTRAINT_CATALOG AND rc.CONSTRAINT_SCHEMA = tc.CONSTRAINT_SCHEMA AND rc.CONSTRAINT_NAME = tc.CONSTRAINT_NAME
LEFT JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE AS kcu2 ON kcu2.CONSTRAINT_CATALOG = rc.UNIQUE_CONSTRAINT_CATALOG AND kcu2.CONSTRAINT_SCHEMA = rc.UNIQUE_CONSTRAINT_SCHEMA AND kcu2.CONSTRAINT_NAME = rc.UNIQUE_CONSTRAINT_NAME AND kcu2.ORDINAL_POSITION = kcu.ORDINAL_POSITION
ORDER BY tc.TABLE_SCHEMA, tc.TABLE_NAME, tc.CONSTRAINT_TYPE, kcu.CONSTRAINT_NAME, kcu.ORDINAL_POSITION
`

type constraintsReader struct {
	rows *sql.Rows
}

func (s constraintsReader) NextConstraint() (constraint schemer.Constraint, err error) {
	if !s.rows.Next() {
		err = s.rows.Err()
		if err != nil {
			err = fmt.Errorf("failed to retrive constaint record: %w", err)
		}
		return
	}
	constraint.Constraint = new(models.Constraint)
	if err = s.rows.Scan(
		&constraint.SchemaName, &constraint.TableName,
		&constraint.Type, &constraint.Name,
		&constraint.ColumnName,
		&constraint.UniqueConstraintCatalog, &constraint.UniqueConstraintSchema, &constraint.UniqueConstraintName,
		&constraint.MatchOption, &constraint.UpdateRule, &constraint.DeleteRule,
		&constraint.RefTableCatalog, &constraint.RefTableSchema, &constraint.RefTableName, &constraint.RefColName,
	); err != nil {
		err = fmt.Errorf("failed to scan constaints record: %w", err)
		return
	}
	return
}
