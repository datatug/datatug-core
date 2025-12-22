package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsKnownCollectionType(t *testing.T) {
	type args struct {
		v CollectionType
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "unknown", args: args{v: CollectionTypeUnknown}, want: false},
		{name: "table", args: args{v: CollectionTypeTable}, want: true},
		{name: "view", args: args{v: CollectionTypeView}, want: true},
		{name: "any", args: args{v: CollectionTypeAny}, want: true},
		{name: "invalid", args: args{v: "invalid"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, IsKnownCollectionType(tt.args.v), "IsKnownCollectionType(%v)", tt.args.v)
		})
	}
}

func TestNewTableKey(t *testing.T) {
	key := NewTableKey("table1", "schema1", "catalog1", nil)
	assert.Equal(t, CollectionTypeTable, key.Type())
	assert.Equal(t, "table1", key.Name())
	assert.Equal(t, "schema1", key.Schema())
	assert.Equal(t, "catalog1", key.Catalog())
}

func TestNewViewKey(t *testing.T) {
	key := NewViewKey("view1", "schema1", "catalog1", nil)
	assert.Equal(t, CollectionTypeView, key.Type())
	assert.Equal(t, "view1", key.Name())
	assert.Equal(t, "schema1", key.Schema())
	assert.Equal(t, "catalog1", key.Catalog())
}

func TestNewCollectionKey(t *testing.T) {
	t.Run("table", func(t *testing.T) {
		key := NewCollectionKey(CollectionTypeTable, "table1", "schema1", "catalog1", nil)
		assert.Equal(t, CollectionTypeTable, key.Type())
		assert.Equal(t, "table1", key.Name())
		assert.Equal(t, "schema1", key.Schema())
		assert.Equal(t, "catalog1", key.Catalog())
	})
	t.Run("view", func(t *testing.T) {
		key := NewCollectionKey(CollectionTypeView, "view1", "schema1", "catalog1", nil)
		assert.Equal(t, CollectionTypeView, key.Type())
		assert.Equal(t, "view1", key.Name())
	})
	t.Run("any", func(t *testing.T) {
		key := NewCollectionKey(CollectionTypeAny, "any1", "schema1", "catalog1", nil)
		assert.Equal(t, CollectionType(CollectionTypeAny), key.Type())
		assert.Equal(t, "any1", key.Name())
	})
	t.Run("invalid_panic", func(t *testing.T) {
		assert.Panics(t, func() {
			NewCollectionKey("invalid", "name", "schema", "catalog", nil)
		})
	})
}

func TestCollectionKey_String(t *testing.T) {
	key := NewTableKey("table1", "schema1", "catalog1", nil)
	assert.NotEmpty(t, key.String())
}

func TestCollectionKey_Validate(t *testing.T) {
	key := NewTableKey("table1", "schema1", "catalog1", nil)
	assert.Nil(t, key.Validate())
}
