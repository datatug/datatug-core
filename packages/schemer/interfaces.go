package schemer

import (
	"context"
	"github.com/datatug/datatug/packages/models"
)

type Scanner interface {
	ScanCatalog(c context.Context, name string) (database *models.Database, err error)
}

type SchemaProvider interface {
	ObjectsProvider
	ColumnsProvider
	IndexesProvider
	IndexColumnsProvider
	ConstraintsProvider
}

type TableRef struct {
	SchemaName string
	TableName  string
	TableType  string
}

type ObjectsProvider interface {
	Objects(c context.Context, catalog string) (ObjectsReader, error)
}

type ObjectsReader interface {
	NextObject() (*models.Table, error)
}

type ColumnsProvider interface {
	Columns(c context.Context, catalog string) (ColumnsReader, error)
}

type ColumnsReader interface {
	NextColumn() (Column, error)
}

type Column struct {
	TableRef
	models.TableColumn
}

type IndexesProvider interface {
	Indexes(c context.Context, catalog string) (IndexesReader, error)
}

type IndexesReader interface {
	NextIndex() (Index, error)
}

type Index struct {
	TableRef
	*models.Index
}

type IndexColumnsProvider interface {
	IndexColumns(c context.Context, catalog string) (IndexColumnsReader, error)
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
	Constraints(c context.Context, catalog string) (ConstraintsReader, error)
}

type ConstraintsReader interface {
	NextConstraint() (Constraint, error)
}

type Constraint struct {
	TableRef
	models.Constraint
}
