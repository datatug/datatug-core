package schemer

import (
	"context"
	"database/sql"
	"github.com/datatug/datatug/packages/models"
)

type Scanner interface {
	ScanCatalog(c context.Context, db *sql.DB, name string) (database *models.DbCatalog, err error)
}

type SchemaProvider interface {
	IsBulkProvider() bool
	TablesProvider
	ColumnsProvider
	IndexesProvider
	IndexColumnsProvider
	ConstraintsProvider
	RecordsCountProvider
}

type TableRef struct {
	SchemaName string
	TableName  string
	TableType  string
}

type TablesProvider interface {
	GetTables(c context.Context, db *sql.DB, catalog, schema string) (TablesReader, error)
}

type TablesReader interface {
	NextTable() (*models.Table, error)
}

type ColumnsProvider interface {
	GetColumns(c context.Context, db *sql.DB, catalog, schemaName, tableName string) (ColumnsReader, error)
}

type ColumnsReader interface {
	NextColumn() (Column, error)
}

type Column struct {
	TableRef
	models.TableColumn
}

type IndexesProvider interface {
	GetIndexes(c context.Context, db *sql.DB, catalog, schema, table string) (IndexesReader, error)
}

type IndexesReader interface {
	NextIndex() (Index, error)
}

type Index struct {
	TableRef
	*models.Index
}

type IndexColumnsProvider interface {
	GetIndexColumns(c context.Context, db *sql.DB, catalog, schema, table string) (IndexColumnsReader, error)
}

type IndexColumnsReader interface {
	NextIndexColumn() (IndexColumn, error)
}

type IndexColumn struct {
	TableRef
	IndexName string
	*models.IndexColumn
}

type ConstraintsProvider interface {
	GetConstraints(c context.Context, db *sql.DB, catalog, schema, table string) (ConstraintsReader, error)
}

type ConstraintsReader interface {
	NextConstraint() (Constraint, error)
}

type Constraint struct {
	TableRef
	ColumnName                                                            string
	UniqueConstraintCatalog, UniqueConstraintSchema, UniqueConstraintName sql.NullString
	MatchOption, UpdateRule, DeleteRule                                   sql.NullString
	RefTableCatalog, RefTableSchema, RefTableName, RefColName             sql.NullString
	*models.Constraint
}

type RecordsCountProvider interface {
	RecordsCount(c context.Context, db *sql.DB, catalog, schema, table string) (*int, error)
}
