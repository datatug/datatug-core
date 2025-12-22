package datatug

import (
	"fmt"

	"github.com/dal-go/dalgo/dal"
)

type CollectionType string

const (
	CollectionTypeAny                    = "*"
	CollectionTypeUnknown CollectionType = ""
	CollectionTypeTable   CollectionType = "table"
	CollectionTypeView    CollectionType = "view"
)

func IsKnownCollectionType(v CollectionType) bool {
	switch v {
	case CollectionTypeAny, CollectionTypeTable, CollectionTypeView:
		return true
	case CollectionTypeUnknown:
		return false
	default:
		return false
	}
}

// DBCollectionKey defines a key that identifies a table or a view
type DBCollectionKey struct {
	schema  string
	catalog string
	t       CollectionType
	Ref     dal.CollectionRef
}

func NewCollectionKey(t CollectionType, name, schema, catalog string, parent *dal.Key) DBCollectionKey {
	if !IsKnownCollectionType(t) {
		panic(fmt.Sprintf("unknown collection type: %s", t))
	}
	return DBCollectionKey{
		t:       t,
		schema:  schema,
		catalog: catalog,
		Ref:     dal.NewCollectionRef(name, "", parent),
	}
}

func NewTableKey(name, schema, catalog string, parent *dal.Key) DBCollectionKey {
	return NewCollectionKey(CollectionTypeTable, name, schema, catalog, parent)
}

func NewViewKey(name, schema, catalog string, parent *dal.Key) DBCollectionKey {
	return NewCollectionKey(CollectionTypeView, name, schema, catalog, parent)
}

func (v DBCollectionKey) Name() string {
	return v.Ref.Name()
}

func (v DBCollectionKey) Type() CollectionType {
	return v.t
}

func (v DBCollectionKey) Schema() string {
	return v.schema
}

func (v DBCollectionKey) Catalog() string {
	return v.catalog
}

func (v DBCollectionKey) String() string {
	return fmt.Sprintf("DBCollectionKey{catalog=%s,ref:%s}", v.catalog, v.Ref.String())
}

// Validate returns error if not valid
func (v DBCollectionKey) Validate() error {
	return nil
}
