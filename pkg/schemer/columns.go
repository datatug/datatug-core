package schemer

import (
	"context"
	"regexp"

	"github.com/dal-go/dalgo/dal"
	"github.com/datatug/datatug-core/pkg/datatug"
)

type ColumnsFilter struct {
	CollectionRef *dal.CollectionRef
	ColNameRegex  *regexp.Regexp
}

// ColumnsProvider reads columns info
type ColumnsProvider interface {
	GetColumnsReader(c context.Context, catalog string, filter ColumnsFilter) (ColumnsReader, error)
	GetColumns(c context.Context, catalog string, filter ColumnsFilter) ([]Column, error)
}

// ColumnsReader provides columns
type ColumnsReader interface {
	// NextColumn should return io.EOF when no more columns
	NextColumn() (Column, error)
}

// Column defines column
type Column struct {
	TableRef
	datatug.ColumnInfo
}
