package datatug2md

import (
	"bytes"
	"net/url"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestTableToReadmeFull(t *testing.T) {
	encoder := NewEncoder()

	repo := &datatug.ProjectRepository{
		WebURL:    "https://github.com/test/repo",
		ProjectID: "test-proj",
	}

	catalog := "test-catalog"

	// Create a referring table
	referringTable := &datatug.CollectionInfo{
		DBCollectionKey: datatug.NewTableKey("ref_table", "dbo", "test-catalog", nil),
		RecordsetBaseDef: datatug.RecordsetBaseDef{
			PrimaryKey: &datatug.UniqueKey{
				Name:    "PK_ref",
				Columns: []string{"id", "id2"},
			},
		},
		TableProps: datatug.TableProps{
			DbType: "BASE TABLE",
		},
	}

	// Create the main table
	recordsCount := 123
	table := &datatug.CollectionInfo{
		DBCollectionKey: datatug.NewTableKey("main_table", "dbo", "test-catalog", nil),
		RecordsCount:    &recordsCount,
		RecordsetBaseDef: datatug.RecordsetBaseDef{
			PrimaryKey: &datatug.UniqueKey{
				Name:    "PK_main",
				Columns: []string{"id1", "id2"},
			},
			ForeignKeys: datatug.ForeignKeys{
				{
					Name:     "FK_main_ref",
					Columns:  []string{"col_with_fk", "col_with_index"},
					RefTable: datatug.NewTableKey("ref_table", "dbo", "test-catalog", nil),
				},
			},
		},
		TableProps: datatug.TableProps{
			DbType: "BASE TABLE",
		},
		Columns: datatug.TableColumns{
			{
				DbColumnProps: datatug.DbColumnProps{
					Name:   "id1",
					DbType: "int",
				},
			},
			{
				DbColumnProps: datatug.DbColumnProps{
					Name:   "id2",
					DbType: "int",
				},
			},
			{
				DbColumnProps: datatug.DbColumnProps{
					Name:   "col_with_index",
					DbType: "varchar(50)",
				},
			},
			{
				DbColumnProps: datatug.DbColumnProps{
					Name:   "col_with_fk",
					DbType: "int",
				},
			},
		},
		Indexes: []*datatug.Index{
			{
				Name:               "IX_test",
				Type:               "NONCLUSTERED",
				Columns:            []*datatug.IndexColumn{{Name: "col_with_index", IsDescending: true}},
				IsUnique:           true,
				IsUniqueConstraint: true,
				IsHash:             true,
				IsXML:              true,
				IsColumnStore:      true,
			},
			{
				Name:         "PK_main",
				Columns:      []*datatug.IndexColumn{{Name: "id1"}, {Name: "id2"}},
				IsPrimaryKey: true,
			},
		},
		ReferencedBy: datatug.TableReferencedBys{
			{
				DBCollectionKey: datatug.NewTableKey("ref_table", "dbo", "test-catalog", nil),
				ForeignKeys: []*datatug.RefByForeignKey{
					{
						Name:    "FK_ref_main",
						Columns: []string{"main_id1", "main_id2"},
					},
				},
			},
		},
	}

	dbServer := datatug.ProjDbServer{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "test-server"}},
		Catalogs: datatug.EnvDbCatalogs{
			{
				DbCatalogBase: datatug.DbCatalogBase{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "test-catalog"}}},
				Schemas: datatug.DbSchemas{
					{
						ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "dbo"}},
						Tables: []*datatug.CollectionInfo{
							table,
							referringTable,
						},
					},
				},
			},
		},
	}

	t.Run("success", func(t *testing.T) {
		w := new(bytes.Buffer)
		err := encoder.TableToReadme(w, repo, catalog, table, dbServer)
		assert.Nil(t, err)
		assert.NotEmpty(t, w.String())
	})

	t.Run("invalid_repo_url", func(t *testing.T) {
		invalidRepo := &datatug.ProjectRepository{
			WebURL: " ://invalid",
		}
		w := new(bytes.Buffer)
		err := encoder.TableToReadme(w, invalidRepo, catalog, table, dbServer)
		assert.NotNil(t, err)
	})

	t.Run("unknown_ref_table", func(t *testing.T) {
		tableWithUnknownFK := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("table_unknown_fk", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
			RecordsetBaseDef: datatug.RecordsetBaseDef{
				ForeignKeys: datatug.ForeignKeys{
					{
						Name:     "FK_unknown",
						RefTable: datatug.NewTableKey("unknown", "dbo", "test-catalog", nil),
					},
				},
			},
		}
		w := new(bytes.Buffer)
		err := encoder.TableToReadme(w, repo, catalog, tableWithUnknownFK, dbServer)
		assert.NotNil(t, err)
	})

	t.Run("unknown_referring_table", func(t *testing.T) {
		tableWithUnknownRefBy := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("table_unknown_ref_by", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
			ReferencedBy: datatug.TableReferencedBys{
				{
					DBCollectionKey: datatug.NewTableKey("unknown", "dbo", "test-catalog", nil),
				},
			},
		}
		w := new(bytes.Buffer)
		err := encoder.TableToReadme(w, repo, catalog, tableWithUnknownRefBy, dbServer)
		assert.NotNil(t, err)
	})

	t.Run("self_referencing_fk", func(t *testing.T) {
		selfRefTable := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("self_ref", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
			RecordsetBaseDef: datatug.RecordsetBaseDef{
				PrimaryKey: &datatug.UniqueKey{
					Name:    "PK_self",
					Columns: []string{"id"},
				},
				ForeignKeys: datatug.ForeignKeys{
					{
						Name:     "FK_self",
						Columns:  []string{"parent_id"},
						RefTable: datatug.NewTableKey("self_ref", "dbo", "test-catalog", nil),
					},
				},
			},
		}
		// Update dbServer to include selfRefTable
		dbServer.Catalogs[0].Schemas[0].Tables = append(dbServer.Catalogs[0].Schemas[0].Tables, selfRefTable)

		w := new(bytes.Buffer)
		err := encoder.TableToReadme(w, repo, catalog, selfRefTable, dbServer)
		assert.Nil(t, err)
	})

	t.Run("complex_names", func(t *testing.T) {
		complexTable := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("!!!", "???", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
		}
		w := new(bytes.Buffer)
		err := encoder.TableToReadme(w, repo, catalog, complexTable, dbServer)
		assert.Nil(t, err)
		assert.Contains(t, w.String(), "[???]")
		assert.Contains(t, w.String(), "[!!!]")
	})

	t.Run("short_name_alias", func(t *testing.T) {
		shortNameTable := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("MyLongTableName", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
		}
		w := new(bytes.Buffer)
		err := encoder.TableToReadme(w, repo, catalog, shortNameTable, dbServer)
		assert.Nil(t, err)
		unescaped, _ := url.QueryUnescape(w.String())
		assert.Contains(t, unescaped, "AS mltn")
	})

	t.Run("short_name_no_alias", func(t *testing.T) {
		shortNameTable := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("short", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
		}
		w := new(bytes.Buffer)
		err := encoder.TableToReadme(w, repo, catalog, shortNameTable, dbServer)
		assert.Nil(t, err)
		assert.NotContains(t, w.String(), "AS ")
	})

	t.Run("recursive_referenced_by", func(t *testing.T) {
		tableA := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("tableA", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
			RecordsetBaseDef: datatug.RecordsetBaseDef{
				PrimaryKey: &datatug.UniqueKey{Name: "PK_A", Columns: []string{"id", "id2"}},
			},
		}
		tableB := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("tableB", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
			RecordsetBaseDef: datatug.RecordsetBaseDef{
				PrimaryKey: &datatug.UniqueKey{Name: "PK_B", Columns: []string{"id"}},
			},
			ReferencedBy: datatug.TableReferencedBys{
				{
					DBCollectionKey: datatug.NewTableKey("tableA", "dbo", "test-catalog", nil),
					ForeignKeys: []*datatug.RefByForeignKey{
						{Name: "FK_AB", Columns: []string{"b_id", "b_id2"}},
					},
				},
			},
		}
		tableA.ReferencedBy = datatug.TableReferencedBys{
			{
				DBCollectionKey: datatug.NewTableKey("tableB", "dbo", "test-catalog", nil),
				ForeignKeys: []*datatug.RefByForeignKey{
					{Name: "FK_BA", Columns: []string{"a_id"}},
				},
			},
		}

		dbServer.Catalogs[0].Schemas[0].Tables = append(dbServer.Catalogs[0].Schemas[0].Tables, tableA, tableB)

		w := new(bytes.Buffer)
		err := encoder.TableToReadme(w, repo, catalog, tableA, dbServer)
		assert.Nil(t, err)
	})

	t.Run("no_repo_app_link", func(t *testing.T) {
		w := new(bytes.Buffer)
		err := encoder.TableToReadme(w, nil, catalog, table, dbServer)
		assert.Nil(t, err)
	})

	t.Run("no_repo_referenced_by", func(t *testing.T) {
		tableWithRefBy := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("tableWithRefBy", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
			RecordsetBaseDef: datatug.RecordsetBaseDef{
				PrimaryKey: &datatug.UniqueKey{Name: "PK", Columns: []string{"col1"}},
			},
			ReferencedBy: datatug.TableReferencedBys{
				{
					DBCollectionKey: datatug.NewTableKey("tableWithRefBy", "dbo", "test-catalog", nil),
					ForeignKeys: []*datatug.RefByForeignKey{
						{Name: "FK_self", Columns: []string{"col1"}},
					},
				},
			},
		}
		dbServer.Catalogs[0].Schemas[0].Tables = append(dbServer.Catalogs[0].Schemas[0].Tables, tableWithRefBy)
		w := new(bytes.Buffer)
		err := encoder.TableToReadme(w, repo, catalog, tableWithRefBy, dbServer)
		assert.Nil(t, err)
		unescaped, _ := url.QueryUnescape(w.String())
		assert.Contains(t, unescaped, "dbo.tableWithRefBy.col1 = dbo.tableWithRefBy.col1")
	})

	t.Run("multiple_foreign_keys_same_column", func(t *testing.T) {
		tableMultiFK := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("multi_fk", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
			RecordsetBaseDef: datatug.RecordsetBaseDef{
				ForeignKeys: datatug.ForeignKeys{
					{
						Name:     "FK1",
						Columns:  []string{"col1"},
						RefTable: datatug.NewTableKey("ref_table", "dbo", "test-catalog", nil),
					},
					{
						Name:     "FK2",
						Columns:  []string{"col1"},
						RefTable: datatug.NewTableKey("ref_table", "dbo", "test-catalog", nil),
					},
				},
			},
			Columns: datatug.TableColumns{
				{DbColumnProps: datatug.DbColumnProps{Name: "col1", DbType: "int"}},
			},
		}
		w := new(bytes.Buffer)
		err := encoder.TableToReadme(w, repo, catalog, tableMultiFK, dbServer)
		assert.Nil(t, err)
	})

	t.Run("nested_referenced_by", func(t *testing.T) {
		tableC := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("tableC", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
			RecordsetBaseDef: datatug.RecordsetBaseDef{
				PrimaryKey: &datatug.UniqueKey{Name: "PK_C", Columns: []string{"id"}},
			},
		}
		tableD := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("tableD", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
			RecordsetBaseDef: datatug.RecordsetBaseDef{
				PrimaryKey: &datatug.UniqueKey{Name: "PK_D", Columns: []string{"id"}},
			},
			ReferencedBy: datatug.TableReferencedBys{
				{
					DBCollectionKey: datatug.NewTableKey("tableC", "dbo", "test-catalog", nil),
					ForeignKeys: []*datatug.RefByForeignKey{
						{Name: "FK_CD", Columns: []string{"d_id"}},
					},
				},
			},
		}
		tableE := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("tableE", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
			RecordsetBaseDef: datatug.RecordsetBaseDef{
				PrimaryKey: &datatug.UniqueKey{Name: "PK_E", Columns: []string{"id"}},
			},
			ReferencedBy: datatug.TableReferencedBys{
				{
					DBCollectionKey: datatug.NewTableKey("tableD", "dbo", "test-catalog", nil),
					ForeignKeys: []*datatug.RefByForeignKey{
						{Name: "FK_DE", Columns: []string{"e_id"}},
					},
				},
			},
		}
		dbServer.Catalogs[0].Schemas[0].Tables = append(dbServer.Catalogs[0].Schemas[0].Tables, tableC, tableD, tableE)
		w := new(bytes.Buffer)
		err := encoder.TableToReadme(w, repo, catalog, tableE, dbServer)
		assert.Nil(t, err)
		assert.Contains(t, w.String(), "Referenced by:")
	})

	t.Run("self_referencing_ref_by_same_name", func(t *testing.T) {
		selfRefTable := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("self_ref", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
			RecordsetBaseDef: datatug.RecordsetBaseDef{
				PrimaryKey: &datatug.UniqueKey{Name: "PK", Columns: []string{"id"}},
			},
			ReferencedBy: datatug.TableReferencedBys{
				{
					DBCollectionKey: datatug.NewTableKey("self_ref", "dbo", "test-catalog", nil),
					ForeignKeys: []*datatug.RefByForeignKey{
						{Name: "FK_self", Columns: []string{"parent_id"}},
					},
				},
			},
		}
		dbServer.Catalogs[0].Schemas[0].Tables = append(dbServer.Catalogs[0].Schemas[0].Tables, selfRefTable)
		w := new(bytes.Buffer)
		err := encoder.TableToReadme(w, repo, catalog, selfRefTable, dbServer)
		assert.Nil(t, err)

		// Self-referencing with DIFFERENT names
		otherTable := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("other_table", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
		}
		selfRefTable2 := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("self_ref_diff", "dbo", "test-catalog", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
			RecordsetBaseDef: datatug.RecordsetBaseDef{
				PrimaryKey: &datatug.UniqueKey{Name: "PK", Columns: []string{"id"}},
			},
			ReferencedBy: datatug.TableReferencedBys{
				{
					DBCollectionKey: datatug.NewTableKey("other_table", "dbo", "test-catalog", nil),
					ForeignKeys: []*datatug.RefByForeignKey{
						{Name: "FK_diff", Columns: []string{"parent_id"}},
					},
				},
			},
		}
		dbServer.Catalogs[0].Schemas[0].Tables = append(dbServer.Catalogs[0].Schemas[0].Tables, otherTable, selfRefTable2)
		w = new(bytes.Buffer)
		err = encoder.TableToReadme(w, repo, catalog, selfRefTable2, dbServer)
		assert.Nil(t, err)
	})
}

func TestWriteReadmeError(t *testing.T) {
	t.Run("parse_error", func(t *testing.T) {
		err := writeReadme(nil, "non-existent.md", nil)
		assert.NotNil(t, err)
	})
}
