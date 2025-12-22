package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCatalogObject_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := CatalogObject{Type: "table", Name: "t1"}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_missing_type", func(t *testing.T) {
		v := CatalogObject{Name: "t1"}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_missing_name", func(t *testing.T) {
		v := CatalogObject{Type: "table"}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_alias", func(t *testing.T) {
		v := CatalogObject{Type: "table", Name: "t1", DefaultAlias: "t1"}
		assert.Error(t, v.Validate())
	})
}

func TestCatalogObjects_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := CatalogObjects{{Type: "table", Name: "t1"}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := CatalogObjects{{}}
		assert.Error(t, v.Validate())
	})
}

func TestDbSchema_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := DbSchema{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := DbSchema{}
		assert.Error(t, v.Validate())
	})
}

func TestDbSchemas_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := DbSchemas{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := DbSchemas{{}}
		assert.Error(t, v.Validate())
	})
}

func TestDbSchemas_GetByID(t *testing.T) {
	s1 := &DbSchema{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}}}
	v := DbSchemas{s1}
	assert.Equal(t, s1, v.GetByID("s1"))
	assert.Nil(t, v.GetByID("s2"))
}

func TestDbCatalogs_GetDbByID(t *testing.T) {
	c1 := &DbCatalog{DbCatalogBase: DbCatalogBase{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "c1"}}}}
	v := DbCatalogs{c1}
	assert.Equal(t, c1, v.GetDbByID("c1"))
	assert.Nil(t, v.GetDbByID("c2"))
}

func TestTableProps_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := TableProps{DbType: "BASE TABLE"}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_empty_type", func(t *testing.T) {
		v := TableProps{}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_unknown_type", func(t *testing.T) {
		v := TableProps{DbType: "INVALID"}
		assert.Error(t, v.Validate())
	})
}

func TestIndex_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := Index{Name: "i1", Type: "BTREE", Columns: []*IndexColumn{{Name: "c1"}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_missing_name", func(t *testing.T) {
		v := Index{Type: "BTREE", Columns: []*IndexColumn{{Name: "c1"}}}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_missing_type", func(t *testing.T) {
		v := Index{Name: "i1", Columns: []*IndexColumn{{Name: "c1"}}}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_missing_columns", func(t *testing.T) {
		v := Index{Name: "i1", Type: "BTREE"}
		assert.Error(t, v.Validate())
	})
}

func TestForeignKey_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := ForeignKey{Name: "fk1", Columns: []string{"c1"}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_missing_name", func(t *testing.T) {
		v := ForeignKey{Columns: []string{"c1"}}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_missing_columns", func(t *testing.T) {
		v := ForeignKey{Name: "fk1"}
		assert.Error(t, v.Validate())
	})
}

func TestCollectionInfo_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := CollectionInfo{
			TableProps:    TableProps{DbType: "BASE TABLE"},
			CollectionKey: NewTableKey("t1", "s1", "c1", nil),
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := CollectionInfo{}
		assert.Error(t, v.Validate())
	})
}

func TestColumnInfo_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := ColumnInfo{
			DbColumnProps: DbColumnProps{Name: "c1", OrdinalPosition: 1},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := ColumnInfo{}
		assert.Error(t, v.Validate())
	})
}
