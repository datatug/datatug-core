package schemer

import (
	"context"

	"github.com/dal-go/dalgo/dal"
	"github.com/datatug/datatug-core/pkg/datatug"
)

// Scanner defines scanner
type Scanner interface {
	ScanCatalog(c context.Context, name string) (database *datatug.DbCatalog, err error)
}

// SchemaProvider provides schema info
type SchemaProvider interface {
	IsBulkProvider() bool // TODO: Needs clarification what it does and how it is used
	CollectionsProvider
	ColumnsProvider
	IndexesProvider
	ForeignKeysProvider
	ReferrersProvider
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

const (
	CatalogsCollection = "catalogs"
	SchemasCollection  = "schemas"
)

func NewSchemaKey(catalog, schema string) *dal.Key {
	catalogKey := dal.NewKeyWithID(CatalogsCollection, catalog)
	return dal.NewKeyWithParentAndID(catalogKey, SchemasCollection, schema)
}

// CollectionsProvider provides Tables
type CollectionsProvider interface {
	// GetCollections returns root collections if parentKey is nil or sub-collection if parenKey is provided
	GetCollections(c context.Context /*db *sql.DB,*/, parentKey *dal.Key) (CollectionsReader, error)
}

// CollectionsReader reads collection info
type CollectionsReader interface {
	// NextCollection returns io.EOF when no more collections
	NextCollection() (*datatug.CollectionInfo, error)
}

// IndexesProvider provides indexes
type IndexesProvider interface {
	// GetIndexes returns next index
	GetIndexes(c context.Context, catalog, schema, table string) (IndexesReader, error)
}

// IndexesReader provides indexes
type IndexesReader interface {
	// NextIndex returns next index
	NextIndex() (*Index, error)
}

// Index defines index
type Index struct {
	TableRef
	*datatug.Index
}

// IndexColumnsProvider provides index columns
type IndexColumnsProvider interface {
	// GetIndexColumns returns index columns
	GetIndexColumns(c context.Context, catalog, schema, table, index string) (IndexColumnsReader, error)
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
	*datatug.IndexColumn
}

// ConstraintsProvider provides constraints
type ConstraintsProvider interface {
	// GetConstraints returns constrains
	GetConstraints(c context.Context, catalog, schema, table string) (ConstraintsReader, error)
}

// ConstraintsReader reads constraint
type ConstraintsReader interface {
	NextConstraint() (*Constraint, error)
}

// Constraint defines a constraint
type Constraint struct {
	TableRef
	ColumnName                                                            string
	UniqueConstraintCatalog, UniqueConstraintSchema, UniqueConstraintName string // can be null
	MatchOption, UpdateRule, DeleteRule                                   string // can be null
	RefTableCatalog, RefTableSchema, RefTableName, RefColName             string // can be null
	*datatug.Constraint
}

// RecordsCountProvider provides count for a recordset
type RecordsCountProvider interface {
	RecordsCount(c context.Context, catalog, schema, table string) (*int, error)
}
