package datatug

import (
	"fmt"

	"github.com/dal-go/dalgo/dal"
)

type CollectionType int

const (
	CollectionTypeUnknown CollectionType = iota
	CollectionTypeTable
	CollectionTypeView
)

// CollectionKey defines a key that identifies a table or a view
type CollectionKey struct {
	schema  string
	catalog string
	t       CollectionType
	Ref     dal.CollectionRef
}

func NewCollectionKey(t CollectionType, name, schema, catalog string, parent *dal.Key) CollectionKey {
	return CollectionKey{
		t:       t,
		schema:  schema,
		catalog: catalog,
		Ref:     dal.NewCollectionRef(name, "", parent),
	}
}
func NewTableKey(name, schema, catalog string, parent *dal.Key) CollectionKey {
	return NewCollectionKey(CollectionTypeTable, name, schema, catalog, parent)
}

func (v CollectionKey) Name() string {
	return v.Ref.Name()
}

func (v CollectionKey) Type() CollectionType {
	return v.t
}

func (v CollectionKey) Schema() string {
	return v.schema
}

func (v CollectionKey) Catalog() string {
	return v.catalog
}

func (v CollectionKey) String() string {
	return fmt.Sprintf("CollectionKey{catalog=%s,ref:%s}", v.catalog, v.Ref.String())
}

// Validate returns error if not valid
func (v CollectionKey) Validate() error {
	return nil
}
