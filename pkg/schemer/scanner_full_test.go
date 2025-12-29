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
	type testCase struct {
		name        string
		provider    *mockSchemaProvider
		ctx         context.Context
		wantErr     bool
		errContains string
	}

	run := func(t *testing.T, tt testCase) {
		if tt.ctx == nil {
			tt.ctx = context.Background()
		}
		scanner := NewScanner(tt.provider)
		_, err := scanner.ScanCatalog(tt.ctx, "cat")
		if (err != nil) != tt.wantErr {
			t.Errorf("ScanCatalog() error = %v, wantErr %v", err, tt.wantErr)
			return
		}
		if tt.wantErr && tt.errContains != "" && (err == nil || !strings.Contains(err.Error(), tt.errContains)) {
			t.Errorf("ScanCatalog() error = %v, want error containing %q", err, tt.errContains)
		}
	}

	errorTests := []testCase{
		{
			name: "UnknownDbType",
			provider: &mockSchemaProvider{
				collections: []*datatug.CollectionInfo{
					{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "UNKNOWN"}},
				},
			},
			wantErr:     true,
			errContains: "unknown DB type",
		},
		{
			name: "UnknownTableInBulkColumns",
			provider: &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				columns:     []Column{{TableRef: TableRef{SchemaName: "s1", TableName: "unknown"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}}},
			},
			wantErr: true,
		},
		{
			name: "ScanCatalog_DeadlineExceeded",
			ctx: func() context.Context {
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
				_ = cancel
				return ctx
			}(),
			provider: &mockSchemaProvider{
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
			},
			wantErr:     true,
			errContains: "exceeded deadline",
		},
		{
			name: "ScanCatalog_GetCollectionsError",
			provider: &mockSchemaProvider{
				getCollectionsErr: errors.New("get collections error"),
			},
			wantErr:     true,
			errContains: "get collections error",
		},
		{
			name: "ScanCatalog_NextCollectionError",
			provider: &mockSchemaProvider{
				nextCollectionErr: errors.New("next collection error"),
			},
			wantErr:     true,
			errContains: "next collection error",
		},
		{
			name: "ScanColumnsInBulk_GetColumnsReaderError",
			provider: &mockSchemaProvider{
				isBulk:        true,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getColumnsErr: errors.New("get columns error"),
			},
			wantErr:     true,
			errContains: "get columns error",
		},
		{
			name: "ScanColumnsInBulk_NextColumnError",
			provider: &mockSchemaProvider{
				isBulk:        true,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextColumnErr: errors.New("next column error"),
			},
			wantErr:     true,
			errContains: "next column error",
		},
		{
			name: "ScanTableCols_GetColumnsReaderError",
			provider: &mockSchemaProvider{
				isBulk:        false,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getColumnsErr: errors.New("get columns error"),
			},
			wantErr:     true,
			errContains: "get columns error",
		},
		{
			name: "ScanIndexesInBulk_GetIndexesError",
			provider: &mockSchemaProvider{
				isBulk:        true,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getIndexesErr: errors.New("get indexes error"),
			},
			wantErr:     true,
			errContains: "get indexes error",
		},
		{
			name: "ScanTableIndexes_GetIndexesError",
			provider: &mockSchemaProvider{
				isBulk:        false,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getIndexesErr: errors.New("get indexes error"),
			},
			wantErr:     true,
			errContains: "get indexes error",
		},
		{
			name: "ScanConstraintsInBulk_GetConstraintsError",
			provider: &mockSchemaProvider{
				isBulk:            true,
				collections:       []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getConstraintsErr: errors.New("get constraints error"),
			},
			wantErr:     true,
			errContains: "get constraints error",
		},
		{
			name: "ScanTableConstraints_GetConstraintsError",
			provider: &mockSchemaProvider{
				isBulk:            false,
				collections:       []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				getConstraintsErr: errors.New("get constraints error"),
			},
			wantErr:     true,
			errContains: "get constraints error",
		},
		{
			name: "ProcessConstraint_ForeignKey_RefTableNotFound",
			provider: &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				constraints: []*Constraint{
					{
						TableRef:        TableRef{SchemaName: "s1", TableName: "t1"},
						Constraint:      &datatug.Constraint{Name: "fk1", Type: "FOREIGN KEY"},
						RefTableCatalog: "cat", RefTableSchema: "s1", RefTableName: "unknown",
					},
				},
			},
			wantErr:     true,
			errContains: "reference table not found",
		},
		{
			name: "ScanConstraintsInBulk_UnknownTable",
			provider: &mockSchemaProvider{
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
			},
			wantErr:     true,
			errContains: "unknown table referenced by constraint",
		},
		{
			name: "ScanIndexesInBulk_NextIndexError",
			provider: &mockSchemaProvider{
				isBulk:       true,
				collections:  []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextIndexErr: errors.New("next index error"),
			},
			wantErr:     true,
			errContains: "next index error",
		},
		{
			name: "ScanIndexesInBulk_UnknownTable",
			provider: &mockSchemaProvider{
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
			},
			wantErr:     true,
			errContains: "unknown table referenced by constraint",
		},
		{
			name: "ScanIndexesInBulk_EmptyIndexName",
			provider: &mockSchemaProvider{
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
			},
			wantErr:     true,
			errContains: "got index with an empty name",
		},
		{
			name: "ScanTableCols_NextColumnError",
			provider: &mockSchemaProvider{
				isBulk:        false,
				collections:   []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextColumnErr: errors.New("next column error"),
			},
			wantErr:     true,
			errContains: "next column error",
		},
		{
			name: "ScanTableIndexes_NextIndexError_NonBulk",
			provider: &mockSchemaProvider{
				isBulk:       false,
				collections:  []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextIndexErr: errors.New("next index error"),
			},
			wantErr:     true,
			errContains: "next index error",
		},
		{
			name: "ScanTableIndexes_NextIndexError_Bulk",
			provider: &mockSchemaProvider{
				isBulk:       true,
				collections:  []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextIndexErr: errors.New("next index error"),
			},
			wantErr:     true,
			errContains: "next index error",
		},
		{
			name: "ScanIndexColumns_NextIndexColumnError_NonBulk",
			provider: &mockSchemaProvider{
				isBulk:             false,
				collections:        []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:            []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				nextIndexColumnErr: errors.New("next index column error"),
			},
			wantErr:     true,
			errContains: "next index column error",
		},
		{
			name: "ScanIndexColumns_NextIndexColumnError_Bulk",
			provider: &mockSchemaProvider{
				isBulk:             true,
				collections:        []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:            []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				nextIndexColumnErr: errors.New("next index column error"),
			},
			wantErr:     true,
			errContains: "next index column error",
		},
		{
			name: "ScanConstraintsInBulk_NextConstraintError",
			provider: &mockSchemaProvider{
				isBulk:            true,
				collections:       []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextConstraintErr: errors.New("next constraint error"),
			},
			wantErr:     true,
			errContains: "next constraint error",
		},
		{
			name: "ScanTableConstraints_NextConstraintError_NonBulk",
			provider: &mockSchemaProvider{
				isBulk:            false,
				collections:       []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextConstraintErr: errors.New("next constraint error"),
			},
			wantErr:     true,
			errContains: "next constraint error",
		},
		{
			name: "ScanTableConstraints_NextConstraintError_Bulk",
			provider: &mockSchemaProvider{
				isBulk:            true,
				collections:       []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				nextConstraintErr: errors.New("next constraint error"),
			},
			wantErr:     true,
			errContains: "next constraint error",
		},
		{
			name: "ScanIndexColumnsInBulk_NextIndexColumnError",
			provider: &mockSchemaProvider{
				isBulk:             true,
				collections:        []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:            []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				nextIndexColumnErr: errors.New("next index column error"),
			},
			wantErr:     true,
			errContains: "next index column error",
		},
		{
			name: "ScanIndexColumnsInBulk_GetIndexColumnsError",
			provider: &mockSchemaProvider{
				isBulk:             true,
				collections:        []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:            []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				getIndexColumnsErr: errors.New("get index columns error"),
			},
			wantErr:     true,
			errContains: "get index columns error",
		},
		{
			name: "ScanIndexColumnsInBulk_UnknownIndex",
			provider: &mockSchemaProvider{
				isBulk:      true,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:     []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				indexCols: []*IndexColumn{
					{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, IndexName: "unknown", IndexColumn: &datatug.IndexColumn{Name: "col1"}},
				},
			},
			wantErr:     true,
			errContains: "unknown index referenced by column",
		},
		{
			name: "ScanTableCols_Deadline",
			ctx: func() context.Context {
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
				_ = cancel
				return ctx
			}(),
			provider: &mockSchemaProvider{
				isBulk:      false,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
			},
			wantErr:     true,
			errContains: "exceeded deadline",
		},
		{
			name: "ScanTableIndexes_Deadline",
			ctx: func() context.Context {
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
				_ = cancel
				return ctx
			}(),
			provider: &mockSchemaProvider{
				isBulk:      false,
				collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				columns:     []Column{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}}},
			},
			wantErr:     true,
			errContains: "exceeded deadline",
		},
		{
			name: "ScanIndexColumns_GetIndexColumnsError_NonBulk",
			provider: &mockSchemaProvider{
				isBulk:             false,
				collections:        []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
				indexes:            []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
				getIndexColumnsErr: errors.New("get index columns error"),
			},
			wantErr:     true,
			errContains: "get index columns error",
		},
		{
			name: "ScanTables_RecordsCountError",
			provider: &mockSchemaProvider{
				collections: []*datatug.CollectionInfo{
					{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}},
				},
				recordsCountErr: errors.New("records count error"),
			},
			wantErr: false, // RecordsCount error is logged but doesn't stop scanning
		},
	}

	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			run(t, tt)
		})
	}

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

	t.Run("ProcessConstraint_UnknownType", func(t *testing.T) {
		table := &datatug.CollectionInfo{DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil)}
		constraint := &Constraint{Constraint: &datatug.Constraint{Type: "UNKNOWN"}}
		err := processConstraint("c1", table, constraint, nil)
		if err != nil {
			t.Errorf("processConstraint should not fail on unknown type, got %v", err)
		}
	})

	t.Run("ScanTables_ContextCancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		provider := &mockSchemaProvider{
			getCollectionsErr: ctx.Err(),
		}
		scanner := NewScanner(provider)
		_, err := scanner.ScanCatalog(ctx, "cat")
		if err == nil {
			t.Error("expected error for cancelled context")
		}
	})

	t.Run("ScanTables_View", func(t *testing.T) {
		provider := &mockSchemaProvider{
			collections: []*datatug.CollectionInfo{
				{DBCollectionKey: datatug.NewTableKey("v1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "VIEW"}},
			},
		}
		scanner := NewScanner(provider)
		catalog, err := scanner.ScanCatalog(context.Background(), "cat")
		if err != nil {
			t.Fatalf("ScanCatalog failed: %v", err)
		}
		if len(catalog.Schemas.GetByID("s1").Views) != 1 {
			t.Errorf("expected 1 view, got %v", len(catalog.Schemas.GetByID("s1").Views))
		}
	})

	t.Run("ForeignKeyProcessing_CompositePK_Reference", func(t *testing.T) {
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
					ColumnName:      "c2",
					RefTableCatalog: catalogID, RefTableSchema: schemaID, RefTableName: "t2",
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
		if t1.ForeignKeys[0].RefTable.Name() != "t2" {
			t.Errorf("expected ref table t2, got %v", t1.ForeignKeys[0].RefTable.Name())
		}
		if len(t1.ForeignKeys[0].Columns) != 2 {
			t.Errorf("expected 2 columns in FK, got %v", len(t1.ForeignKeys[0].Columns))
		}
	})

	t.Run("ScanIndexColumns_Deadline_NonBulk", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
		defer cancel()
		provider := &mockSchemaProvider{
			isBulk:      false,
			collections: []*datatug.CollectionInfo{{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil), TableProps: datatug.TableProps{DbType: "BASE TABLE"}}},
			columns:     []Column{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}}},
			indexes:     []*Index{{TableRef: TableRef{SchemaName: "s1", TableName: "t1"}, Index: &datatug.Index{Name: "idx1"}}},
		}
		scanner := NewScanner(provider)
		_, err := scanner.ScanCatalog(ctx, "cat")
		if err == nil || !strings.Contains(err.Error(), "exceeded deadline") {
			t.Errorf("expected deadline error, got %v", err)
		}
	})

	t.Run("ProcessConstraint_Unique_Multiple", func(t *testing.T) {
		table := &datatug.CollectionInfo{DBCollectionKey: datatug.NewTableKey("t1", "s1", "cat", nil)}
		u1 := &Constraint{Constraint: &datatug.Constraint{Name: "u1", Type: "UNIQUE"}, ColumnName: "col1"}
		u2 := &Constraint{Constraint: &datatug.Constraint{Name: "u2", Type: "UNIQUE"}, ColumnName: "col2"}
		_ = processConstraint("cat", table, u1, nil)
		_ = processConstraint("cat", table, u2, nil)
		if len(table.AlternateKeys) != 2 {
			t.Errorf("expected 2 alternate keys, got %v", len(table.AlternateKeys))
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
			t.Errorf("expected error for unknown DB type, got %v", err)
		}
	})
}
