package schemer

import "context"

type FKAnchor struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns,omitempty"`
}

// ForeignKey describes a foreign key
type ForeignKey struct {
	Name string   `json:"name,omitempty"`
	From FKAnchor `json:"from"`
	To   FKAnchor `json:"to"`
}

type ForeignKeysReader interface {
	// NextForeignKey should return io.EOF when finished
	NextForeignKey() (ForeignKey, error)
}

type ForeignKeysProvider interface {
	GetForeignKeysReader(c context.Context, schema, table string) (ForeignKeysReader, error)
	GetForeignKeys(c context.Context, schema, table string) ([]ForeignKey, error)
}
