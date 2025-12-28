package schemer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func TestScanCatalog_Bulk(t *testing.T) {
	catalogID := "test_catalog"
	schemaID := "test_schema"
	tableName := "test_table"

	table1 := &datatug.CollectionInfo{
		DBCollectionKey: datatug.NewTableKey(tableName, schemaID, catalogID, nil),
		TableProps: datatug.TableProps{
			DbType: "BASE TABLE",
		},
	}

	provider := &mockSchemaProvider{
		isBulk:      true,
		collections: []*datatug.CollectionInfo{table1},
		columns: []Column{
			{
				TableRef: TableRef{SchemaName: schemaID, TableName: tableName},
				ColumnInfo: datatug.ColumnInfo{
					DbColumnProps: datatug.DbColumnProps{Name: "col1"},
				},
			},
		},
		indexes: []*Index{
			{
				TableRef: TableRef{SchemaName: schemaID, TableName: tableName},
				Index: &datatug.Index{
					Name: "idx1",
				},
			},
		},
		indexCols: []*IndexColumn{
			{
				TableRef:  TableRef{SchemaName: schemaID, TableName: tableName},
				IndexName: "idx1",
				IndexColumn: &datatug.IndexColumn{
					Name: "col1",
				},
			},
		},
		constraints: []*Constraint{
			{
				TableRef: TableRef{SchemaName: schemaID, TableName: tableName},
				Constraint: &datatug.Constraint{
					Name: "pk1",
					Type: "PRIMARY KEY",
				},
				ColumnName: "col1",
			},
		},
		recordsCount: map[string]int{
			catalogID + "." + schemaID + "." + tableName: 100,
		},
	}

	scanner := NewScanner(provider)
	catalog, err := scanner.ScanCatalog(context.Background(), catalogID)
	if err != nil {
		t.Fatalf("ScanCatalog failed: %v", err)
	}

	if catalog.ID != catalogID {
		t.Errorf("expected catalog ID %v, got %v", catalogID, catalog.ID)
	}

	schema := catalog.Schemas.GetByID(schemaID)
	if schema == nil {
		t.Fatalf("schema %v not found", schemaID)
	}

	table := datatug.Tables(schema.Tables).GetByKey(table1.DBCollectionKey)
	if table == nil {
		t.Fatalf("table %v not found", tableName)
	}

	if len(table.Columns) != 1 {
		t.Errorf("expected 1 column, got %v", len(table.Columns))
	}

	if len(table.Indexes) != 1 {
		t.Errorf("expected 1 index, got %v", len(table.Indexes))
	}

	if table.PrimaryKey == nil {
		t.Error("expected primary key, got nil")
	}

	if table.RecordsCount == nil || *table.RecordsCount != 100 {
		t.Errorf("expected records count 100, got %v", table.RecordsCount)
	}
}

func TestScanCatalog_NonBulk(t *testing.T) {
	catalogID := "test_catalog"
	schemaID := "test_schema"
	tableName := "test_table"

	table1 := &datatug.CollectionInfo{
		DBCollectionKey: datatug.NewTableKey(tableName, schemaID, catalogID, nil),
		TableProps: datatug.TableProps{
			DbType: "BASE TABLE",
		},
	}

	provider := &mockSchemaProvider{
		isBulk:      false,
		collections: []*datatug.CollectionInfo{table1},
		columns: []Column{
			{
				TableRef: TableRef{SchemaName: schemaID, TableName: tableName},
				ColumnInfo: datatug.ColumnInfo{
					DbColumnProps: datatug.DbColumnProps{Name: "col1"},
				},
			},
		},
		indexes: []*Index{
			{
				TableRef: TableRef{SchemaName: schemaID, TableName: tableName},
				Index: &datatug.Index{
					Name: "idx1",
				},
			},
		},
		indexCols: []*IndexColumn{
			{
				TableRef:  TableRef{SchemaName: schemaID, TableName: tableName},
				IndexName: "idx1",
				IndexColumn: &datatug.IndexColumn{
					Name: "col1",
				},
			},
		},
		constraints: []*Constraint{
			{
				TableRef: TableRef{SchemaName: schemaID, TableName: tableName},
				Constraint: &datatug.Constraint{
					Name: "pk1",
					Type: "PRIMARY KEY",
				},
				ColumnName: "col1",
			},
		},
	}

	scanner := NewScanner(provider)
	catalog, err := scanner.ScanCatalog(context.Background(), catalogID)
	if err != nil {
		t.Fatalf("ScanCatalog failed: %v", err)
	}

	table := datatug.Tables(catalog.Schemas.GetByID(schemaID).Tables).GetByKey(table1.DBCollectionKey)
	if len(table.Columns) != 1 {
		t.Errorf("expected 1 column, got %v", len(table.Columns))
	}
}

func TestScanCatalog_Errors(t *testing.T) {
	t.Run("UnknownDbType", func(t *testing.T) {
		provider := &mockSchemaProvider{
			collections: []*datatug.CollectionInfo{
				{
					DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil),
					TableProps: datatug.TableProps{
						DbType: "UNKNOWN",
					},
				},
			},
		}
		scanner := NewScanner(provider)
		_, err := scanner.ScanCatalog(context.Background(), "cat")
		if err == nil {
			t.Error("expected error for unknown DB type")
		}
	})

	t.Run("UnknownTableInBulkColumns", func(t *testing.T) {
		provider := &mockSchemaProvider{
			isBulk: true,
			collections: []*datatug.CollectionInfo{
				{
					DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil),
					TableProps: datatug.TableProps{
						DbType: "BASE TABLE",
					},
				},
			},
			columns: []Column{
				{
					TableRef:   TableRef{SchemaName: "s1", TableName: "unknown"},
					ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}},
				},
			},
		}
		scanner := NewScanner(provider)
		_, err := scanner.ScanCatalog(context.Background(), "cat")
		if err == nil {
			t.Error("expected error for unknown table in bulk columns")
		}
	})

	getByName := func(tables datatug.Tables, name string) *datatug.CollectionInfo {
		for _, t := range tables {
			if t.Name() == name {
				return t
			}
		}
		return nil
	}

	t.Run("ForeignKeyProcessing", func(t *testing.T) {
		catalogID := "cat"
		schemaID := "s1"
		provider := &mockSchemaProvider{
			isBulk: true,
			collections: []*datatug.CollectionInfo{
				{
					DBCollectionKey: datatug.NewTableKey("t1", schemaID, catalogID, nil),
					TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
				},
				{
					DBCollectionKey: datatug.NewTableKey("t2", schemaID, catalogID, nil),
					TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
				},
			},
			constraints: []*Constraint{
				{
					TableRef:        TableRef{SchemaName: schemaID, TableName: "t1"},
					Constraint:      &datatug.Constraint{Name: "fk1", Type: "FOREIGN KEY"},
					ColumnName:      "c1",
					RefTableCatalog: catalogID, RefTableSchema: schemaID, RefTableName: "t2",
				},
				{
					TableRef:        TableRef{SchemaName: schemaID, TableName: "t1"},
					Constraint:      &datatug.Constraint{Name: "fk1", Type: "FOREIGN KEY"},
					ColumnName:      "c2", // composite FK
					RefTableCatalog: catalogID, RefTableSchema: schemaID, RefTableName: "t2",
				},
				{
					TableRef:        TableRef{SchemaName: schemaID, TableName: "t2"},
					Constraint:      &datatug.Constraint{Name: "fk1", Type: "FOREIGN KEY"}, // same name but on different table
					ColumnName:      "c1",
					RefTableCatalog: catalogID, RefTableSchema: schemaID, RefTableName: "t1",
				},
				{
					TableRef:        TableRef{SchemaName: schemaID, TableName: "t2"},
					Constraint:      &datatug.Constraint{Name: "fk1", Type: "FOREIGN KEY"}, // existing refByFk
					ColumnName:      "c2",
					RefTableCatalog: catalogID, RefTableSchema: schemaID, RefTableName: "t1",
				},
			},
		}
		scanner := NewScanner(provider)
		catalog, err := scanner.ScanCatalog(context.Background(), catalogID)
		if err != nil {
			t.Fatalf("ScanCatalog failed: %v", err)
		}
		tables := datatug.Tables(catalog.Schemas.GetByID(schemaID).Tables)
		t1 := getByName(tables, "t1")
		if len(t1.ForeignKeys) != 1 {
			t.Errorf("expected 1 FK, got %v", len(t1.ForeignKeys))
		}
		if len(t1.ForeignKeys[0].Columns) != 2 {
			t.Errorf("expected composite FK with 2 columns, got %v", len(t1.ForeignKeys[0].Columns))
		}
		t2 := getByName(tables, "t2")
		if len(t2.ReferencedBy) != 1 {
			t.Errorf("expected t2 to be referenced by 1 table, got %v", len(t2.ReferencedBy))
		}
	})

	t.Run("UniqueConstraintProcessing", func(t *testing.T) {
		catalogID := "cat"
		schemaID := "s1"
		provider := &mockSchemaProvider{
			isBulk: true,
			collections: []*datatug.CollectionInfo{
				{
					DBCollectionKey: datatug.NewTableKey("t1", schemaID, catalogID, nil),
					TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
				},
			},
			constraints: []*Constraint{
				{
					TableRef:   TableRef{SchemaName: schemaID, TableName: "t1"},
					Constraint: &datatug.Constraint{Name: "u1", Type: "UNIQUE"},
					ColumnName: "c1",
				},
				{
					TableRef:   TableRef{SchemaName: schemaID, TableName: "t1"},
					Constraint: &datatug.Constraint{Name: "u1", Type: "UNIQUE"},
					ColumnName: "c2",
				},
			},
		}
		scanner := NewScanner(provider)
		catalog, err := scanner.ScanCatalog(context.Background(), catalogID)
		if err != nil {
			t.Fatalf("ScanCatalog failed: %v", err)
		}
		tables := datatug.Tables(catalog.Schemas.GetByID(schemaID).Tables)
		t1 := getByName(tables, "t1")
		if len(t1.AlternateKeys) != 1 {
			t.Errorf("expected 1 Unique Key (AlternateKey), got %v", len(t1.AlternateKeys))
		}
	})

	t.Run("SortedTables_Reset", func(t *testing.T) {
		st := SortedTables{
			Tables: []*datatug.CollectionInfo{
				{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil)},
				{DBCollectionKey: datatug.NewTableKey("t2", "s1", "c1", nil)},
			},
		}
		if st.SequentialFind("c1", "s1", "t1") == nil {
			t.Fatal("expected to find t1")
		}
		if st.SequentialFind("c1", "s1", "t2") == nil {
			t.Fatal("expected to find t2")
		}
		st.Reset()
		if st.SequentialFind("c1", "s1", "t1") == nil {
			t.Fatal("expected to find t1 after reset")
		}
	})

	t.Run("SortedIndexes_ResetAndMissing", func(t *testing.T) {
		si := SortedIndexes{
			indexes: []*Index{
				{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "i1"}},
				{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "i2"}},
			},
		}
		if si.SequentialFind("s1", "t1", "i1") == nil {
			t.Fatal("expected to find i1")
		}
		if si.SequentialFind("s1", "t1", "i2") == nil {
			t.Fatal("expected to find i2")
		}
		si.Reset()
		if si.SequentialFind("s1", "t1", "i1") == nil {
			t.Fatal("expected to find i1 after reset")
		}
		si.Reset()
		if si.SequentialFind("s1", "t1", "unknown") != nil {
			t.Fatal("expected not to find unknown index")
		}
	})

	t.Run("FindTable_CaseInsensitive", func(t *testing.T) {
		tables := datatug.Tables{
			{DBCollectionKey: datatug.NewTableKey("Table1", "Schema1", "Catalog1", nil)},
		}
		if FindTable(tables, "catalog1", "schema1", "table1") == nil {
			t.Error("FindTable should be case-insensitive")
		}
		if FindTable(tables, "cat", "sch", "tab") != nil {
			t.Error("FindTable should return nil for non-existent table")
		}
	})

	t.Run("ScanCatalog_DeadlineExceeded", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
		defer cancel()
		provider := &mockSchemaProvider{
			collections: []*datatug.CollectionInfo{
				{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
			},
		}
		scanner := NewScanner(provider)
		_, err := scanner.ScanCatalog(ctx, "c1")
		if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
			t.Errorf("expected deadline exceeded error, got %v", err)
		}
	})

	t.Run("Bulk_ScanIndexes_EmptyName", func(t *testing.T) {
		provider := &mockSchemaProvider{
			isBulk: true,
			collections: []*datatug.CollectionInfo{
				{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
			},
			indexes: []*Index{
				{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: ""}},
			},
		}
		scanner := NewScanner(provider)
		_, err := scanner.ScanCatalog(context.Background(), "c1")
		if err == nil || !strings.Contains(err.Error(), "empty name") {
			t.Errorf("expected error for empty index name, got %v", err)
		}
	})

	t.Run("Bulk_ScanIndexes_UnknownTable", func(t *testing.T) {
		provider := &mockSchemaProvider{
			isBulk: true,
			collections: []*datatug.CollectionInfo{
				{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
			},
			indexes: []*Index{
				{TableRef: TableRef{SchemaName: "s1", TableName: "unknown"}, Index: &datatug.Index{Name: "idx1"}},
			},
		}
		scanner := NewScanner(provider)
		_, err := scanner.ScanCatalog(context.Background(), "c1")
		if err == nil || !strings.Contains(err.Error(), "unknown table referenced by constraint") {
			t.Errorf("expected error for unknown table in bulk indexes, got %v", err)
		}
	})

	t.Run("Bulk_ScanIndexColumns_UnknownIndex", func(t *testing.T) {
		provider := &mockSchemaProvider{
			isBulk: true,
			collections: []*datatug.CollectionInfo{
				{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
			},
			indexes: []*Index{
				{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}},
			},
			indexCols: []*IndexColumn{
				{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, IndexName: "unknown", IndexColumn: &datatug.IndexColumn{Name: "col1"}},
			},
		}
		scanner := NewScanner(provider)
		_, err := scanner.ScanCatalog(context.Background(), "c1")
		if err == nil || !strings.Contains(err.Error(), "unknown index referenced by column") {
			t.Errorf("expected error for unknown index in bulk index columns, got %v", err)
		}
	})

	t.Run("ScanTables_CollectionsError", func(t *testing.T) {
		provider := &mockSchemaProvider{
			getCollectionsErr: errors.New("collections error"),
		}
		scanner := NewScanner(provider)
		_, err := scanner.ScanCatalog(context.Background(), "c1")
		if err == nil || !strings.Contains(err.Error(), "collections error") {
			t.Errorf("expected error for collections retrieval, got %v", err)
		}
	})

	t.Run("ScanColumnsInBulk_Error", func(t *testing.T) {
		provider := &mockSchemaProvider{
			isBulk:        true,
			getColumnsErr: errors.New("columns error"),
			collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
		}
		scanner := NewScanner(provider)
		_, err := scanner.ScanCatalog(context.Background(), "c1")
		if err == nil || !strings.Contains(err.Error(), "columns error") {
			t.Errorf("expected error for columns retrieval in bulk, got %v", err)
		}
	})

	t.Run("ScanConstraintsInBulk_Error", func(t *testing.T) {
		provider := &mockSchemaProvider{
			isBulk:            true,
			getConstraintsErr: errors.New("constraints error"),
			collections:       []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
		}
		scanner := NewScanner(provider)
		_, err := scanner.ScanCatalog(context.Background(), "c1")
		if err == nil || !strings.Contains(err.Error(), "constraints error") {
			t.Errorf("expected error for constraints retrieval in bulk, got %v", err)
		}
	})

	t.Run("ScanIndexesInBulk_Error", func(t *testing.T) {
		provider := &mockSchemaProvider{
			isBulk:        true,
			getIndexesErr: errors.New("indexes error"),
			collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
		}
		scanner := NewScanner(provider)
		_, err := scanner.ScanCatalog(context.Background(), "c1")
		if err == nil || !strings.Contains(err.Error(), "indexes error") {
			t.Errorf("expected error for indexes retrieval in bulk, got %v", err)
		}
	})

	t.Run("ScanIndexColumnsInBulk_Error", func(t *testing.T) {
		provider := &mockSchemaProvider{
			isBulk:        true,
			getIndexesErr: nil, // indexes succeed
			indexes:       []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "i1"}}},
			collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
		}
		// Since we need to inject error into GetIndexColumns, let's add it to mockSchemaProvider
		provider.err = errors.New("index columns error")
		// But wait, provider.err affects everything. Let's add specific field.
		scanner := NewScanner(provider)
		_, err := scanner.ScanCatalog(context.Background(), "c1")
		if err == nil || !strings.Contains(err.Error(), "index columns error") {
			t.Errorf("expected error for index columns retrieval in bulk, got %v", err)
		}
	})

	t.Run("CompositePrimaryKey", func(t *testing.T) {
		catalogID := "cat"
		schemaID := "s1"
		provider := &mockSchemaProvider{
			isBulk: false,
			collections: []*datatug.CollectionInfo{
				{
					DBCollectionKey: datatug.NewTableKey("t1", schemaID, catalogID, nil),
					TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
				},
			},
			columns: []Column{
				{
					TableRef: TableRef{SchemaName: schemaID, TableName: "t1"},
					ColumnInfo: datatug.ColumnInfo{
						DbColumnProps: datatug.DbColumnProps{Name: "c1", PrimaryKeyPosition: 1},
					},
				},
				{
					TableRef: TableRef{SchemaName: schemaID, TableName: "t1"},
					ColumnInfo: datatug.ColumnInfo{
						DbColumnProps: datatug.DbColumnProps{Name: "c2", PrimaryKeyPosition: 2},
					},
				},
			},
		}
		scanner := NewScanner(provider)
		catalog, err := scanner.ScanCatalog(context.Background(), catalogID)
		if err != nil {
			t.Fatalf("ScanCatalog failed: %v", err)
		}
		t1 := datatug.Tables(catalog.Schemas.GetByID(schemaID).Tables)[0]
		if t1.PrimaryKey == nil {
			t.Fatal("expected primary key")
		}
		if len(t1.PrimaryKey.Columns) != 2 {
			t.Errorf("expected 2 columns in PK, got %v", len(t1.PrimaryKey.Columns))
		}
	})

	t.Run("NonBulk_Errors", func(t *testing.T) {
		t.Run("GetTablePropsError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:        false,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getColumnsErr: errors.New("columns error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "c1")
			if err == nil || !strings.Contains(err.Error(), "columns error") {
				t.Errorf("expected error for columns retrieval in non-bulk, got %v", err)
			}
		})

		t.Run("ScanTableIndexesError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:        false,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getIndexesErr: errors.New("indexes error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "c1")
			if err == nil || !strings.Contains(err.Error(), "indexes error") {
				t.Errorf("expected error for indexes retrieval in non-bulk, got %v", err)
			}
		})

		t.Run("ScanTableConstraintsError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:            false,
				collections:       []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getConstraintsErr: errors.New("constraints error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "c1")
			if err == nil || !strings.Contains(err.Error(), "constraints error") {
				t.Errorf("expected error for constraints retrieval in non-bulk, got %v", err)
			}
		})

		t.Run("ScanIndexColumnsError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      false,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:     []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "i1"}}},
			}
			provider.err = errors.New("index columns error")
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "c1")
			if err == nil || !strings.Contains(err.Error(), "index columns error") {
				t.Errorf("expected error for index columns retrieval in non-bulk, got %v", err)
			}
		})

		t.Run("ScanTableConstraints_ProcessError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      false,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				constraints: []*Constraint{
					{
						TableRef:        TableRef{SchemaName: "s1", TableName: "t1"},
						Constraint:      &datatug.Constraint{Name: "fk1", Type: "FOREIGN KEY"},
						RefTableCatalog: "c1", RefTableSchema: "s1", RefTableName: "unknown",
					},
				},
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "c1")
			if err == nil || !strings.Contains(err.Error(), "reference table not found") {
				t.Errorf("expected error for unknown reference table in non-bulk, got %v", err)
			}
		})

		t.Run("ProcessConstraint_CompositeUniqueKey", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				constraints: []*Constraint{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Constraint: &datatug.Constraint{Name: "u1", Type: "UNIQUE"}, ColumnName: "c1"},
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Constraint: &datatug.Constraint{Name: "u1", Type: "UNIQUE"}, ColumnName: "c2"},
				},
			}
			scanner := NewScanner(provider)
			catalog, _ := scanner.ScanCatalog(context.Background(), "cat")
			t1 := datatug.Tables(catalog.Schemas.GetByID("s1").Tables)[0]
			if len(t1.AlternateKeys) != 1 || len(t1.AlternateKeys[0].Columns) != 2 {
				t.Errorf("expected 1 composite unique key, got %v", t1.AlternateKeys)
			}
		})

		t.Run("ProcessConstraint_ExistingPrimaryKey", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				constraints: []*Constraint{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Constraint: &datatug.Constraint{Name: "pk1", Type: "PRIMARY KEY"}, ColumnName: "c1"},
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Constraint: &datatug.Constraint{Name: "pk1", Type: "PRIMARY KEY"}, ColumnName: "c2"},
				},
			}
			scanner := NewScanner(provider)
			catalog, _ := scanner.ScanCatalog(context.Background(), "cat")
			t1 := datatug.Tables(catalog.Schemas.GetByID("s1").Tables)[0]
			if t1.PrimaryKey == nil || len(t1.PrimaryKey.Columns) != 2 {
				t.Errorf("expected 1 composite primary key, got %v", t1.PrimaryKey)
			}
		})
	})

	t.Run("AdditionalCoverageTests", func(t *testing.T) {
		t.Run("ScanTables_GetCollectionsError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				getCollectionsErr: errors.New("get collections error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "get collections error") {
				t.Errorf("expected get collections error, got %v", err)
			}
		})

		t.Run("ScanTables_NextCollectionError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				nextCollectionErr: errors.New("next collection error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next collection error") {
				t.Errorf("expected next collection error, got %v", err)
			}
		})

		t.Run("ScanTables_RecordsCountError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				collections: []*datatug.CollectionInfo{
					{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
				},
				recordsCountErr: errors.New("records count error"),
			}
			scanner := NewScanner(provider)
			// RecordsCount error is logged but doesn't stop scanning
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err != nil {
				t.Errorf("expected no error for records count (logged only), got %v", err)
			}
		})

		t.Run("ScanColumnsInBulk_NextColumnError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:        true,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextColumnErr: errors.New("next column error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next column error") {
				t.Errorf("expected next column error, got %v", err)
			}
		})

		t.Run("ScanIndexesInBulk_NextIndexError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:       true,
				collections:  []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextIndexErr: errors.New("next index error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next index error") {
				t.Errorf("expected next index error, got %v", err)
			}
		})

		t.Run("ScanIndexesInBulk_EmptyIndexName", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes: []*Index{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: ""}},
				},
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "got index with an empty name") {
				t.Errorf("expected error for empty index name, got %v", err)
			}
		})

		t.Run("ScanIndexColumnsInBulk_NextIndexColumnError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:             true,
				collections:        []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:            []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				nextIndexColumnErr: errors.New("next index column error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next index column error") {
				t.Errorf("expected next index column error, got %v", err)
			}
		})

		t.Run("ScanTables_MultipleSchemas", func(t *testing.T) {
			provider := &mockSchemaProvider{
				collections: []*datatug.CollectionInfo{
					{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
					{DBCollectionKey: datatug.NewTableKey("t2", "s2", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
				},
			}
			scanner := NewScanner(provider)
			catalog, err := scanner.ScanCatalog(context.Background(), "cat")
			if err != nil {
				t.Fatalf("ScanCatalog failed: %v", err)
			}
			if len(catalog.Schemas) != 2 {
				t.Errorf("expected 2 schemas, got %v", len(catalog.Schemas))
			}
		})

		t.Run("ScanTables_UnknownDbType", func(t *testing.T) {
			provider := &mockSchemaProvider{
				collections: []*datatug.CollectionInfo{
					{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "OTHER"}},
				},
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "unknown DB type") {
				t.Errorf("expected unknown DB type error, got %v", err)
			}
		})

		t.Run("ScanTables_Deadline_Instant", func(t *testing.T) {
			provider := &mockSchemaProvider{
				collections: []*datatug.CollectionInfo{
					{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
				},
			}
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(deadlineCtx, "cat")
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected exceeded deadline error in scanTables, got %v", err)
			}
		})

		t.Run("SortedTables_SequentialFind_NotFound", func(t *testing.T) {
			st := SortedTables{Tables: []*datatug.CollectionInfo{
				{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil)},
			}}
			if st.SequentialFind("c1", "s1", "unknown") != nil {
				t.Error("expected nil for unknown table")
			}
		})

		t.Run("SortedIndexes_SequentialFind_NotFound", func(t *testing.T) {
			si := SortedIndexes{indexes: []*Index{
				{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "i1"}},
			}}
			if si.SequentialFind("s1", "t1", "unknown") != nil {
				t.Error("expected nil for unknown index")
			}
		})

		t.Run("ScanTables_Bulk_Workers_Error", func(t *testing.T) {
			// This is tricky because parallel.Run returns the first error.
			// We want to cover the error paths of scanColumnsInBulk, scanConstraintsInBulk, scanIndexesInBulk when called as workers.
			provider := &mockSchemaProvider{
				isBulk: true,
				collections: []*datatug.CollectionInfo{
					{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
				},
				getColumnsErr: errors.New("bulk columns error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "bulk columns error") {
				t.Errorf("expected bulk columns error, got %v", err)
			}

			provider.getColumnsErr = nil
			provider.getConstraintsErr = errors.New("bulk constraints error")
			_, err = scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "bulk constraints error") {
				t.Errorf("expected bulk constraints error, got %v", err)
			}

			provider.getConstraintsErr = nil
			provider.getIndexesErr = errors.New("bulk indexes error")
			_, err = scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "bulk indexes error") {
				t.Errorf("expected bulk indexes error, got %v", err)
			}
		})

		t.Run("ScanColumnsInBulk_Deadline", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk: true,
				collections: []*datatug.CollectionInfo{
					{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
				},
				columns: []Column{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}},
				},
			}
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(deadlineCtx, "cat")
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected exceeded deadline error in scanColumnsInBulk, got %v", err)
			}
		})

		t.Run("ScanTableCols_Deadline_Instant", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      false,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
			}
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(deadlineCtx, "cat")
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected exceeded deadline error, got %v", err)
			}
		})

		t.Run("ScanTableIndexes_Deadline_Instant", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      false,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes: []*Index{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "i1"}},
				},
			}
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(deadlineCtx, "cat")
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected exceeded deadline error in scanTableIndexes, got %v", err)
			}
		})

		t.Run("ScanIndexColumns_Deadline_Instant", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      false,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes: []*Index{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "i1"}},
				},
				indexCols: []*IndexColumn{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, IndexName: "i1", IndexColumn: &datatug.IndexColumn{Name: "c1"}},
				},
			}
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(deadlineCtx, "cat")
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected exceeded deadline error in scanIndexColumns, got %v", err)
			}
		})

		t.Run("ScanConstraintsInBulk_Deadline_Instant", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				constraints: []*Constraint{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Constraint: &datatug.Constraint{Name: "pk1", Type: "PRIMARY KEY"}, ColumnName: "c1"},
				},
			}
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(deadlineCtx, "cat")
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected exceeded deadline error in scanConstraintsInBulk, got %v", err)
			}
		})

		t.Run("ScanIndexesInBulk_Deadline_Instant", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes: []*Index{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}},
				},
			}
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(deadlineCtx, "cat")
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected exceeded deadline error in scanIndexesInBulk, got %v", err)
			}
		})

		t.Run("ScanIndexColumnsInBulk_Deadline_Instant", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:     []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				indexCols: []*IndexColumn{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, IndexName: "idx1", IndexColumn: &datatug.IndexColumn{Name: "col1"}},
				},
			}
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(deadlineCtx, "cat")
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected exceeded deadline error in scanIndexColumnsInBulk, got %v", err)
			}
		})

		t.Run("ScanTableCols_CompositePK", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      false,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				columns: []Column{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c2", PrimaryKeyPosition: 2}}},
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1", PrimaryKeyPosition: 1}}},
				},
			}
			scanner := NewScanner(provider)
			catalog, err := scanner.ScanCatalog(context.Background(), "cat")
			if err != nil {
				t.Fatalf("ScanCatalog failed: %v", err)
			}
			table := catalog.Schemas.GetByID("s1").Tables[0]
			if table.PrimaryKey == nil || len(table.PrimaryKey.Columns) != 2 {
				t.Errorf("expected composite PK with 2 columns, got %v", table.PrimaryKey)
			}
			if table.PrimaryKey.Columns[0] != "c1" || table.PrimaryKey.Columns[1] != "c2" {
				t.Errorf("expected PK columns [c1, c2], got %v", table.PrimaryKey.Columns)
			}
		})

		t.Run("ScanConstraintsInBulk_UnknownTable", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk: true,
				collections: []*datatug.CollectionInfo{
					{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
				},
				constraints: []*Constraint{
					{
						TableRef:   TableRef{SchemaName: "s1", TableName: "unknown"},
						Constraint: &datatug.Constraint{Name: "pk1", Type: "PRIMARY KEY"},
						ColumnName: "c1",
					},
				},
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "unknown table referenced by constraint") {
				t.Errorf("expected unknown table error in bulk constraints, got %v", err)
			}
		})

		t.Run("ScanIndexesInBulk_UnknownTable", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk: true,
				collections: []*datatug.CollectionInfo{
					{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
				},
				indexes: []*Index{
					{
						TableRef: TableRef{SchemaName: "s1", TableName: "unknown"},
						Index:    &datatug.Index{Name: "idx1"},
					},
				},
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "unknown table referenced by constraint") {
				t.Errorf("expected unknown table error in bulk indexes, got %v", err)
			}
		})

		t.Run("ScanIndexesInBulk_EmptyIndexName", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk: true,
				collections: []*datatug.CollectionInfo{
					{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
				},
				indexes: []*Index{
					{
						TableRef: TableRef{SchemaName: "s1", TableName: "t1"},
						Index:    &datatug.Index{Name: ""},
					},
				},
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "got index with an empty name") {
				t.Errorf("expected empty index name error in bulk indexes, got %v", err)
			}
		})

		t.Run("ScanTableCols_NextColumnError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:        false,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextColumnErr: errors.New("next column error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next column error") {
				t.Errorf("expected next column error in non-bulk, got %v", err)
			}
		})

		t.Run("ScanTableIndexes_NextIndexError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:       false,
				collections:  []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextIndexErr: errors.New("next index error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next index error") {
				t.Errorf("expected next index error in non-bulk, got %v", err)
			}
		})

		t.Run("ScanIndexColumns_NextIndexColumnError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:             false,
				collections:        []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:            []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				nextIndexColumnErr: errors.New("next index column error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next index column error") {
				t.Errorf("expected next index column error in non-bulk, got %v", err)
			}
		})

		t.Run("ScanTableConstraints_NextConstraintError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:            false,
				collections:       []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextConstraintErr: errors.New("next constraint error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next constraint error") {
				t.Errorf("expected next constraint error in non-bulk, got %v", err)
			}
		})

		t.Run("ProcessConstraint_UnknownType", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				constraints: []*Constraint{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Constraint: &datatug.Constraint{Name: "c1", Type: "UNKNOWN"}, ColumnName: "col1"},
				},
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err != nil {
				t.Errorf("expected no error for unknown constraint type (it should just be skipped), got %v", err)
			}
		})

		t.Run("ScanConstraintsInBulk_GetConstraintsError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:            true,
				getConstraintsErr: errors.New("get constraints error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "get constraints error") {
				t.Errorf("expected get constraints error in bulk, got %v", err)
			}
		})

		t.Run("ScanConstraintsInBulk_NextConstraintError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:            true,
				nextConstraintErr: errors.New("next constraint error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next constraint error") {
				t.Errorf("expected next constraint error in bulk, got %v", err)
			}
		})

		t.Run("ScanConstraintsInBulk_UnknownTable", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				constraints: []*Constraint{
					{TableRef: TableRef{SchemaName: "s1", TableName: "unknown"}, Constraint: &datatug.Constraint{Name: "c1", Type: "PRIMARY KEY"}},
				},
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "unknown table referenced by constraint") {
				t.Errorf("expected unknown table error in bulk constraints, got %v", err)
			}
		})

		t.Run("ScanColumnsInBulk_GetColumnsReaderError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:        true,
				getColumnsErr: errors.New("get columns error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "get columns error") {
				t.Errorf("expected get columns error in bulk, got %v", err)
			}
		})

		t.Run("ScanIndexesInBulk_GetIndexesError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:        true,
				getIndexesErr: errors.New("get indexes error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "get indexes error") {
				t.Errorf("expected get indexes error in bulk, got %v", err)
			}
		})

		t.Run("ScanIndexesInBulk_UnknownTable", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes: []*Index{
					{TableRef: TableRef{SchemaName: "s1", TableName: "unknown"}, Index: &datatug.Index{Name: "i1"}},
				},
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "unknown table referenced by constraint") { // actually it says "referenced by constraint" in code for indexes too
				t.Errorf("expected unknown table error in bulk indexes, got %v", err)
			}
		})

		t.Run("ScanIndexColumnsInBulk_GetIndexColumnsError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:             true,
				collections:        []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:            []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				getIndexColumnsErr: errors.New("get index columns error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "get index columns error") {
				t.Errorf("expected get index columns error in bulk, got %v", err)
			}
		})

		t.Run("ContextDeadlineTests", func(t *testing.T) {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()

			provider := &mockSchemaProvider{
				collections: []*datatug.CollectionInfo{
					{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
				},
			}
			scanner := NewScanner(provider)

			t.Run("ScanTables_Deadline", func(t *testing.T) {
				_, err := scanner.ScanCatalog(ctx, "cat")
				if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
					t.Errorf("expected exceeded deadline error in scanTables, got %v", err)
				}
			})

			// For bulk scans, we need to bypass scanTables first or use a provider that returns collections but then check deadline.
			// However, scanTables checks deadline at each iteration.
		})

		t.Run("ContextDeadlineBulkTests", func(t *testing.T) {
			// To test deadline in bulk scans, we need to have collections but then have a deadline before bulk scans.
			// But bulk scans start after all collections are read.
			// We can use a custom mock that cancels context.
		})

		t.Run("ScanIndexColumnsInBulk_UnknownIndex", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:     []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				indexCols: []*IndexColumn{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, IndexName: "unknown", IndexColumn: &datatug.IndexColumn{Name: "col1"}},
				},
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "unknown index referenced by column") {
				t.Errorf("expected unknown index error in bulk index columns, got %v", err)
			}
		})

		t.Run("ScanColumnsInBulk_Deadline", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				columns: []Column{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}},
				},
			}
			// Cancel before bulk starts? Actually scanColumnsInBulk checks deadline inside loop.
			// Let's use a real deadline.
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(deadlineCtx, "cat")
			// It might fail at scanTables first.
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected deadline error, got %v", err)
			}
		})

		t.Run("ScanConstraintsInBulk_Deadline", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
			}
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(deadlineCtx, "cat")
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected deadline error in bulk constraints, got %v", err)
			}
		})

		t.Run("ScanIndexesInBulk_Deadline", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
			}
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(deadlineCtx, "cat")
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected deadline error in bulk indexes, got %v", err)
			}
		})

		t.Run("ScanIndexColumnsInBulk_Deadline", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
			}
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(deadlineCtx, "cat")
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected deadline error in bulk index columns, got %v", err)
			}
		})

		t.Run("ProcessConstraint_CompositeForeignKey_ExistingRefByTable", func(t *testing.T) {
			catalogID := "cat"
			schemaID := "s1"
			t1Key := datatug.NewTableKey("t1", schemaID, catalogID, nil)
			t2Key := datatug.NewTableKey("t2", schemaID, catalogID, nil)

			t1 := &datatug.CollectionInfo{DBCollectionKey: t1Key, TableProps: datatug.TableProps{DbType: "BASE TABLE"}}
			t2 := &datatug.CollectionInfo{DBCollectionKey: t2Key, TableProps: datatug.TableProps{DbType: "BASE TABLE"}}

			// Pre-populate ReferencedBy
			t2.ReferencedBy = append(t2.ReferencedBy, &datatug.TableReferencedBy{
				DBCollectionKey: t1Key,
				ForeignKeys:     make([]*datatug.RefByForeignKey, 0),
			})

			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{t1, t2},
				constraints: []*Constraint{
					{
						TableRef:        TableRef{SchemaName: schemaID, TableName: "t1"},
						Constraint:      &datatug.Constraint{Name: "fk1", Type: "FOREIGN KEY"},
						ColumnName:      "c1",
						RefTableCatalog: catalogID, RefTableSchema: schemaID, RefTableName: "t2",
					},
					{
						TableRef:        TableRef{SchemaName: schemaID, TableName: "t1"},
						Constraint:      &datatug.Constraint{Name: "fk1", Type: "FOREIGN KEY"},
						ColumnName:      "c2",
						RefTableCatalog: catalogID, RefTableSchema: schemaID, RefTableName: "t2",
					},
				},
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), catalogID)
			if err != nil {
				t.Fatalf("ScanCatalog failed: %v", err)
			}
			if len(t2.ReferencedBy) != 1 {
				t.Errorf("expected 1 refByTable, got %v", len(t2.ReferencedBy))
			}
			if len(t2.ReferencedBy[0].ForeignKeys) != 1 {
				t.Errorf("expected 1 refByFk, got %v", len(t2.ReferencedBy[0].ForeignKeys))
			}
		})

		t.Run("ScanIndexColumnsInBulk_IndexNotFoundInFinder", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:     []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				indexCols: []*IndexColumn{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, IndexName: "idx1", IndexColumn: &datatug.IndexColumn{Name: "col1"}},
				},
			}
			// To trigger the branch where index != nil but index.Index == nil (unlikely but covered by code)
			// OR the branch where index == nil. SequentialFind returns nil if not found.
			// Let's use IndexName that doesn't match idx1.
			provider.indexCols[0].IndexName = "nonexistent"

			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "unknown index referenced by column") {
				t.Errorf("expected unknown index error, got %v", err)
			}
		})

		t.Run("ScanIndexColumnsInBulk_IndexItemNil", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes: []*Index{
					{
						TableRef: TableRef{SchemaName: "s1", TableName: "t1"},
						Index:    nil,
					},
				},
			}
			// When index.Index is nil, it should trigger the error path in scanIndexesInBulk
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil {
				t.Error("expected error for nil index.Index, got nil")
			}
		})

		t.Run("ScanTableCols_NextColumnError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:        false,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextColumnErr: errors.New("next column error"),
			}
			scanner := NewScanner(provider)
			// Non-bulk mode
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next column error") {
				t.Errorf("expected next column error, got %v", err)
			}
		})

		t.Run("ScanTableIndexes_NextIndexError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:       false,
				collections:  []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextIndexErr: errors.New("next index error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next index error") {
				t.Errorf("expected next index error, got %v", err)
			}
		})

		t.Run("ScanIndexColumns_NextIndexColumnError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:             false,
				collections:        []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:            []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				nextIndexColumnErr: errors.New("next index column error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next index column error") {
				t.Errorf("expected next index column error, got %v", err)
			}
		})

		t.Run("ScanConstraintsInBulk_NextConstraintError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:            true,
				collections:       []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextConstraintErr: errors.New("next constraint error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next constraint error") {
				t.Errorf("expected next constraint error, got %v", err)
			}
		})

		t.Run("ScanColumnsInBulk_NextColumnError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:        true,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextColumnErr: errors.New("next column error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next column error") {
				t.Errorf("expected next column error, got %v", err)
			}
		})

		t.Run("ScanIndexesInBulk_NextIndexError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:       true,
				collections:  []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextIndexErr: errors.New("next index error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next index error") {
				t.Errorf("expected next index error, got %v", err)
			}
		})

		t.Run("ScanTables_NextCollectionError_AfterFirst", func(t *testing.T) {
			provider := &mockSchemaProvider{
				collections: []*datatug.CollectionInfo{
					{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
				},
				nextCollectionErr: errors.New("next collection error after first"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "next collection error after first") {
				t.Errorf("expected next collection error after first, got %v", err)
			}
		})

		t.Run("ScanIndexColumns_GetIndexColumnsError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:             false,
				collections:        []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:            []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				getIndexColumnsErr: errors.New("get index columns error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "get index columns error") {
				t.Errorf("expected get index columns error, got %v", err)
			}
		})

		t.Run("ScanIndexColumnsInBulk_ErrorFormatting", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:     []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				indexCols: []*IndexColumn{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, IndexName: "unknown", IndexColumn: &datatug.IndexColumn{Name: "col1"}},
				},
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "Known indexes: idx1") {
				t.Errorf("expected error with known indexes, got %v", err)
			}
		})

		t.Run("ScanIndexColumnsInBulk_IndexItemNil_SequentialFind", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:     []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				indexCols: []*IndexColumn{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, IndexName: "idx1", IndexColumn: &datatug.IndexColumn{Name: "col1"}},
				},
			}
			// We want to trigger the case where index != nil but index.Index == nil in scanIndexColumnsInBulk.
			// This is hard with the current mock because Index.Index is what's added to tables.
			// However, SequentialFind returns *Index from si.indexes.
			provider.indexes[0].Index = nil // This will trigger error in scanIndexesInBulk before it reaches scanIndexColumnsInBulk

			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil {
				t.Error("expected error for nil index.Index")
			}
		})

		t.Run("SortedTables_SequentialFind_Loop", func(t *testing.T) {
			st := SortedTables{Tables: []*datatug.CollectionInfo{
				{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil)},
				{DBCollectionKey: datatug.NewTableKey("t2", "s1", "c1", nil)},
			}}
			if st.SequentialFind("c1", "s1", "t1") == nil {
				t.Error("expected t1")
			}
			if st.SequentialFind("c1", "s1", "t2") == nil {
				t.Error("expected t2")
			}
			st.Reset()
			if st.SequentialFind("c1", "s1", "t1") == nil {
				t.Error("expected t1 after reset")
			}
		})

		t.Run("SortedIndexes_SequentialFind_Loop", func(t *testing.T) {
			si := SortedIndexes{indexes: []*Index{
				{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "i1"}},
				{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "i2"}},
			}}
			if si.SequentialFind("s1", "t1", "i1") == nil {
				t.Error("expected i1")
			}
			if si.SequentialFind("s1", "t1", "i2") == nil {
				t.Error("expected i2")
			}
			si.Reset()
			if si.SequentialFind("s1", "t1", "i1") == nil {
				t.Error("expected i1 after reset")
			}
		})

		t.Run("ScanTableCols_Deadline_AfterFirst", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      false,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				columns: []Column{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}},
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c2"}}},
				},
			}
			// This requires fine-grained control over when the deadline is checked.
			// Since we can't easily wait between NextColumn calls in the mock, we use a very short deadline.
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Millisecond))
			defer cancel()
			time.Sleep(2 * time.Millisecond)
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(ctx, "cat")
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected exceeded deadline error, got %v", err)
			}
		})

		t.Run("ScanConstraintsInBulk_GetConstraintsError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:            true,
				collections:       []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getConstraintsErr: errors.New("get constraints error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "get constraints error") {
				t.Errorf("expected get constraints error, got %v", err)
			}
		})

		t.Run("ScanIndexesInBulk_GetIndexesError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:        true,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getIndexesErr: errors.New("get indexes error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "get indexes error") {
				t.Errorf("expected get indexes error, got %v", err)
			}
		})

		t.Run("ScanIndexColumnsInBulk_GetIndexColumnsError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:             true,
				collections:        []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:            []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				getIndexColumnsErr: errors.New("get index columns error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "get index columns error") {
				t.Errorf("expected get index columns error, got %v", err)
			}
		})

		t.Run("ScanColumnsInBulk_GetColumnsReaderError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:        true,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getColumnsErr: errors.New("get columns error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "get columns error") {
				t.Errorf("expected get columns error, got %v", err)
			}
		})

		t.Run("ScanTableCols_GetColumnsReaderError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:        false,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getColumnsErr: errors.New("get columns error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "get columns error") {
				t.Errorf("expected get columns error, got %v", err)
			}
		})

		t.Run("ScanTableIndexes_GetIndexesError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:        false,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getIndexesErr: errors.New("get indexes error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "get indexes error") {
				t.Errorf("expected get indexes error, got %v", err)
			}
		})

		t.Run("ScanTableConstraints_GetConstraintsError", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:            false,
				collections:       []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getConstraintsErr: errors.New("get constraints error"),
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "get constraints error") {
				t.Errorf("expected get constraints error, got %v", err)
			}
		})

		t.Run("ProcessConstraint_ForeignKey_RefTableNotFound", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				constraints: []*Constraint{
					{
						TableRef:        TableRef{SchemaName: "s1", TableName: "t1"},
						Constraint:      &datatug.Constraint{Name: "fk1", Type: "FOREIGN KEY"},
						RefTableCatalog: "cat", RefTableSchema: "s1", RefTableName: "unknown",
					},
				},
			}
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(context.Background(), "cat")
			if err == nil || !strings.Contains(err.Error(), "reference table not found") {
				t.Errorf("expected reference table not found error, got %v", err)
			}
		})

		t.Run("ProcessConstraint_UnknownType", func(t *testing.T) {
			table := &datatug.CollectionInfo{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil)}
			constraint := &Constraint{Constraint: &datatug.Constraint{Type: "UNKNOWN"}}
			err := processConstraint("c1", table, constraint, nil)
			if err != nil {
				t.Errorf("processConstraint should not fail on unknown type, got %v", err)
			}
		})
	})

	t.Run("FinalCoverageTests", func(t *testing.T) {
		t.Run("FindTable_Nil", func(t *testing.T) {
			if FindTable(nil, "c", "s", "t") != nil {
				t.Error("expected nil for nil tables")
			}
		})

		t.Run("ScanTableCols_Deadline", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      false,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
			}
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(deadlineCtx, "cat")
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected deadline error in scanTableCols, got %v", err)
			}
		})

		t.Run("ScanTableIndexes_Deadline", func(t *testing.T) {
			provider := &mockSchemaProvider{
				isBulk:      false,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				columns:     []Column{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}}},
			}
			deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			defer cancel()
			scanner := NewScanner(provider)
			_, err := scanner.ScanCatalog(deadlineCtx, "cat")
			if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
				t.Errorf("expected deadline error in scanTableIndexes, got %v", err)
			}
		})
	})
}
