package datatug

import (
	"sort"
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

func TestCatalogObjectWithRefs_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := CatalogObjectWithRefs{CatalogObject: CatalogObject{Type: "table", Name: "t1"}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_catalog_object", func(t *testing.T) {
		v := CatalogObjectWithRefs{CatalogObject: CatalogObject{}}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_primary_key", func(t *testing.T) {
		v := CatalogObjectWithRefs{
			CatalogObject: CatalogObject{Type: "table", Name: "t1"},
			PrimaryKey:    &UniqueKey{Name: ""},
		}
		assert.Error(t, v.Validate())
	})
}

func TestCatalogObjectsWithRefs_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := CatalogObjectsWithRefs{{CatalogObject: CatalogObject{Type: "table", Name: "t1"}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := CatalogObjectsWithRefs{{CatalogObject: CatalogObject{}}}
		assert.Error(t, v.Validate())
	})
}

func TestDbSchema_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := DbSchema{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_project_item", func(t *testing.T) {
		v := DbSchema{}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_table", func(t *testing.T) {
		v := DbSchema{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}},
			Tables:      []*CollectionInfo{{}},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_view", func(t *testing.T) {
		v := DbSchema{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}},
			Views:       []*CollectionInfo{{}},
		}
		assert.Error(t, v.Validate())
	})
}

func TestDbCatalogs_GetTable(t *testing.T) {
	v := DbCatalogs{
		{
			DbCatalogBase: DbCatalogBase{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "c1"}}},
			Schemas: DbSchemas{
				{
					ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}},
					Tables: []*CollectionInfo{
						{DBCollectionKey: NewTableKey("t1", "s1", "c1", nil)},
					},
				},
			},
		},
	}
	assert.NotNil(t, v.GetTable("c1", "s1", "t1"))
	assert.Nil(t, v.GetTable("c2", "s1", "t1"))
	assert.Nil(t, v.GetTable("c1", "s2", "t1"))
	assert.Nil(t, v.GetTable("c1", "s1", "t2"))
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

func TestDbCatalogs_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := DbCatalogs{{DbCatalogBase: DbCatalogBase{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "c1"}}, Driver: "mysql"}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_schema", func(t *testing.T) {
		v := DbCatalog{
			DbCatalogBase: DbCatalogBase{
				ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "c1", Title: "T"}},
				Driver:      "postgres",
			},
			Schemas: DbSchemas{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: ""}}}},
		}
		assert.Error(t, v.Validate())
	})
}

func TestTableKeys_Validate(t *testing.T) {
	v := TableKeys{NewTableKey("t1", "s1", "c1", nil)}
	assert.NoError(t, v.Validate())
	// TableKey.Validate always returns nil, so we can't easily test invalid case unless we mock it
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
	t.Run("invalid_unique_key", func(t *testing.T) {
		v := TableProps{
			DbType:     "BASE TABLE",
			UniqueKeys: []*UniqueKey{{Name: ""}},
		}
		assert.Error(t, v.Validate())
	})
}

func TestUniqueKeys_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := UniqueKeys{{Name: "uk1", Columns: []string{"c1"}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := UniqueKeys{{}}
		assert.Error(t, v.Validate())
	})
}

func TestIndexes_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := Indexes{{Name: "i1", Type: "BTREE", Columns: []*IndexColumn{{Name: "c1"}}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := Indexes{{}}
		assert.Error(t, v.Validate())
	})
}

func TestUniqueKey_Validate(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var v *UniqueKey
		assert.NoError(t, v.Validate())
	})
	t.Run("valid", func(t *testing.T) {
		v := &UniqueKey{Name: "uk1", Columns: []string{"c1"}}
		assert.NoError(t, v.Validate())
	})
	t.Run("missing_name", func(t *testing.T) {
		v := &UniqueKey{Columns: []string{"c1"}}
		assert.Error(t, v.Validate())
	})
	t.Run("missing_columns", func(t *testing.T) {
		v := &UniqueKey{Name: "uk1"}
		assert.Error(t, v.Validate())
	})
	t.Run("empty_column_name", func(t *testing.T) {
		v := &UniqueKey{Name: "uk1", Columns: []string{""}}
		assert.Error(t, v.Validate())
	})
}

func TestForeignKeys_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := ForeignKeys{{Name: "fk1", Columns: []string{"c1"}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_no_name", func(t *testing.T) {
		v := ForeignKeys{{Name: "", Columns: []string{"c1"}}}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_with_name", func(t *testing.T) {
		v := ForeignKeys{{Name: "fk1", Columns: nil}}
		assert.Error(t, v.Validate())
	})
}

func TestConstraint_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := Constraint{Name: "c1", Type: "CHECK"}
		assert.NoError(t, v.Validate())
	})
	t.Run("missing_name", func(t *testing.T) {
		v := Constraint{Type: "CHECK"}
		assert.Error(t, v.Validate())
	})
	t.Run("missing_type", func(t *testing.T) {
		v := Constraint{Name: "c1"}
		assert.Error(t, v.Validate())
	})
}

func TestTables_GetByKey(t *testing.T) {
	key := NewTableKey("t1", "s1", "c1", nil)
	v := Tables{{DBCollectionKey: key}}
	assert.NotNil(t, v.GetByKey(key))
	assert.Nil(t, v.GetByKey(NewTableKey("t2", "s1", "c1", nil)))
}

func TestTableReferencedBys_Validate(t *testing.T) {
	v := TableReferencedBys{}
	assert.NoError(t, v.Validate())
}

func TestDbColumnProps_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := DbColumnProps{Name: "c1", OrdinalPosition: 1}
		assert.NoError(t, v.Validate())
	})
	t.Run("missing_name", func(t *testing.T) {
		v := DbColumnProps{OrdinalPosition: 1}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_ordinal", func(t *testing.T) {
		v := DbColumnProps{Name: "c1", OrdinalPosition: -1}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_precision", func(t *testing.T) {
		p := -1
		v := DbColumnProps{Name: "c1", DateTimePrecision: &p}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_char_max", func(t *testing.T) {
		l := -2
		v := DbColumnProps{Name: "c1", CharMaxLength: &l}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_char_octet", func(t *testing.T) {
		l := -2
		v := DbColumnProps{Name: "c1", CharOctetLength: &l}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_charset", func(t *testing.T) {
		v := DbColumnProps{Name: "c1", CharacterSet: &CharacterSet{}}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_collation", func(t *testing.T) {
		v := DbColumnProps{Name: "c1", Collation: &Collation{}}
		assert.Error(t, v.Validate())
	})
}

func TestTableColumns_ByPrimaryKeyPosition(t *testing.T) {
	v := TableColumns{
		{DbColumnProps: DbColumnProps{Name: "c1", PrimaryKeyPosition: 2}},
		{DbColumnProps: DbColumnProps{Name: "c2", PrimaryKeyPosition: 1}},
	}
	sort.Sort(v.ByPrimaryKeyPosition())
	assert.Equal(t, "c2", v[0].Name)
}

func TestTableColumns_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := TableColumns{{DbColumnProps: DbColumnProps{Name: "c1", OrdinalPosition: 1}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := TableColumns{{}}
		assert.Error(t, v.Validate())
	})
}

func TestColumnModel_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := ColumnModel{ColumnInfo: ColumnInfo{DbColumnProps: DbColumnProps{Name: "c1", OrdinalPosition: 1}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_info", func(t *testing.T) {
		v := ColumnModel{ColumnInfo: ColumnInfo{}}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_by_env", func(t *testing.T) {
		v := ColumnModel{
			ColumnInfo: ColumnInfo{DbColumnProps: DbColumnProps{Name: "c1", OrdinalPosition: 1}},
			ByEnv:      StateByEnv{"env1": {Status: "INVALID"}},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_checks", func(t *testing.T) {
		v := ColumnModel{
			ColumnInfo: ColumnInfo{DbColumnProps: DbColumnProps{Name: "c1", OrdinalPosition: 1}},
			Checks:     Checks{{ID: ""}},
		}
		assert.Error(t, v.Validate())
	})
}

func TestColumnModels_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := ColumnModels{{ColumnInfo: ColumnInfo{DbColumnProps: DbColumnProps{Name: "c1", OrdinalPosition: 1}}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := ColumnModels{{ColumnInfo: ColumnInfo{}}}
		assert.Error(t, v.Validate())
	})
}

func TestCollation_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := Collation{Name: "utf8_general_ci"}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := Collation{}
		assert.Error(t, v.Validate())
	})
}

func TestCharacterSet_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := CharacterSet{Name: "utf8"}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := CharacterSet{}
		assert.Error(t, v.Validate())
	})
}

func TestCollectionInfo_Validate_Detailed(t *testing.T) {
	t.Run("invalid_primary_key", func(t *testing.T) {
		v := CollectionInfo{
			TableProps:      TableProps{DbType: "BASE TABLE"},
			DBCollectionKey: NewTableKey("t1", "s1", "c1", nil),
			RecordsetBaseDef: RecordsetBaseDef{
				PrimaryKey: &UniqueKey{Name: ""},
			},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_columns", func(t *testing.T) {
		v := CollectionInfo{
			TableProps:      TableProps{DbType: "BASE TABLE"},
			DBCollectionKey: NewTableKey("t1", "s1", "c1", nil),
			Columns:         TableColumns{{}},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_foreign_keys", func(t *testing.T) {
		v := CollectionInfo{
			TableProps:      TableProps{DbType: "BASE TABLE"},
			DBCollectionKey: NewTableKey("t1", "s1", "c1", nil),
			RecordsetBaseDef: RecordsetBaseDef{
				ForeignKeys: ForeignKeys{{}},
			},
		}
		assert.Error(t, v.Validate())
	})
}
