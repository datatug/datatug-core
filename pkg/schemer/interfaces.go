package schemer

import (
	"context"
	"database/sql"
	"github.com/datatug/datatug-core/pkg/models"
)

// Scanner defines scanner
type Scanner interface {
	ScanCatalog(c context.Context, db *sql.DB, name string) (database *models.DbCatalog, err error)
}

// SchemaProvider provides schema info
type SchemaProvider interface {
	IsBulkProvider() bool
	TablesProvider
	ColumnsProvider
	IndexesProvider
	IndexColumnsProvider
	ConstraintsProvider
	RecordsCountProvider
}

// TableRef defines a reference to a table
type TableRef struct {
	SchemaName string
	TableName  string
	TableType  string
}

// TablesProvider provides tables
type TablesProvider interface {
	// GetTables returns tables
	GetTables(c context.Context, db *sql.DB, catalog, schema string) (TablesReader, error)
}

// TablesReader reads table info
type TablesReader interface {
	NextTable() (*models.Table, error)
}

// ColumnsProvider reads columns info
type ColumnsProvider interface {
	GetColumns(c context.Context, db *sql.DB, catalog, schemaName, tableName string) (ColumnsReader, error)
}

// ColumnsReader provides columns
type ColumnsReader interface {
	// NextColumn returns next column
	NextColumn() (Column, error)
}

// Column defines column
type Column struct {
	TableRef
	models.TableColumn
}

// IndexesProvider provides indexes
type IndexesProvider interface {
	// GetIndexes returns next index
	GetIndexes(c context.Context, db *sql.DB, catalog, schema, table string) (IndexesReader, error)
}

// IndexesReader provides indexes
type IndexesReader interface {
	// NextIndex returns next index
	NextIndex() (*Index, error)
}

// Index defines index
type Index struct {
	TableRef
	*models.Index
}

// IndexColumnsProvider provides index columns
type IndexColumnsProvider interface {
	// GetIndexColumns returns index columns
	GetIndexColumns(c context.Context, db *sql.DB, catalog, schema, table, index string) (IndexColumnsReader, error)
}

// IndexColumnsReader provides index columns
type IndexColumnsReader interface {
	// NextIndexColumn returns index column
	NextIndexColumn() (*IndexColumn, error)
}

// IndexColumn defines index column
type IndexColumn struct {
	TableRef
	IndexName string
	*models.IndexColumn
}

// ConstraintsProvider provides constraints
type ConstraintsProvider interface {
	// GetConstraints returns constrains
	GetConstraints(c context.Context, db *sql.DB, catalog, schema, table string) (ConstraintsReader, error)
}

// ConstraintsReader reads constraint
type ConstraintsReader interface {
	NextConstraint() (*Constraint, error)
}

// Constraint defines a constraint
type Constraint struct {
	TableRef
	ColumnName                                                            string
	UniqueConstraintCatalog, UniqueConstraintSchema, UniqueConstraintName sql.NullString
	MatchOption, UpdateRule, DeleteRule                                   sql.NullString
	RefTableCatalog, RefTableSchema, RefTableName, RefColName             sql.NullString
	*models.Constraint
}

// RecordsCountProvider provides count for a recordset
type RecordsCountProvider interface {
	RecordsCount(c context.Context, db *sql.DB, catalog, schema, table string) (*int, error)
}
