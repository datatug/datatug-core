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

var _ schemer.ConstraintsProvider = (*constraintsProvider)(nil)

type constraintsProvider struct {
}

func (v constraintsProvider) GetConstraints(_ context.Context, db *sql.DB, catalog, schema, table string) (schemer.ConstraintsReader, error) {
	if err := verifyTableParams(catalog, schema, table); err != nil {
		return nil, err
	}
	rows, err := db.Query(constraintsSQL, table)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve constraints: %w", err)
	}
	return constraintsReader{schema: schema, table: table, rows: rows}, nil
}

var _ schemer.ConstraintsReader = (*constraintsReader)(nil)

//go:embed constraints.sql
var constraintsSQL string

type constraintsReader struct {
	schema string
	table  string
	rows   *sql.Rows
}

func (s constraintsReader) NextConstraint() (constraint *schemer.Constraint, err error) {
	if !s.rows.Next() {
		err = s.rows.Err()
		if err != nil {
			err = fmt.Errorf("failed to retrive constaint record: %w", err)
		}
		return
	}
	constraint = new(schemer.Constraint)
	constraint.Constraint = new(models.Constraint)
	constraint.Type = "FOREIGN KEY"

	if err = s.rows.Scan(
		&constraint.RefTableName,
		&constraint.RefColName,
		&constraint.ColumnName,
		&constraint.UpdateRule,
		&constraint.DeleteRule,
		&constraint.MatchOption,
	); err != nil {
		err = fmt.Errorf("failed to scan constraints record: %w", err)
		return
	}
	constraint.Name = fmt.Sprintf("FK_%v2%v", s.table, constraint.RefTableName)
	if constraint.ColumnName == constraint.RefColName.String {
		constraint.Name += "_" + constraint.ColumnName
	}
	constraint.RefTableSchema.Valid = true
	constraint.RefTableSchema.String = s.schema
	constraint.SchemaName = s.schema
	constraint.TableName = s.table
	return
}
