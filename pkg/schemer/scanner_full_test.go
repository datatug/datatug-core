package schemer

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/datatug/datatug-core/pkg/datatug"
)

type mockSchemaProvider struct {
	SchemaProvider
	isBulk             bool
	collectionsReader  CollectionsReader
	columnsReader      ColumnsReader
	indexesReader      IndexesReader
	indexColumnsReader IndexColumnsReader
	constraintsReader  ConstraintsReader
}

func (m *mockSchemaProvider) IsBulkProvider() bool { return m.isBulk }
func (m *mockSchemaProvider) GetCollections(_ context.Context, parentKey *dal.Key) (CollectionsReader, error) {
	_ = parentKey
	return m.collectionsReader, nil
}
func (m *mockSchemaProvider) GetColumnsReader(_ context.Context, catalog string, filter ColumnsFilter) (ColumnsReader, error) {
	_ = catalog
	_ = filter
	return m.columnsReader, nil
}
func (m *mockSchemaProvider) GetIndexes(_ context.Context, catalog, schema, table string) (IndexesReader, error) {
	_, _, _ = catalog, schema, table
	return m.indexesReader, nil
}
func (m *mockSchemaProvider) GetIndexColumns(_ context.Context, catalog, schema, table, index string) (IndexColumnsReader, error) {
	_, _, _, _ = catalog, schema, table, index
	return m.indexColumnsReader, nil
}
func (m *mockSchemaProvider) GetConstraints(_ context.Context, catalog, schema, table string) (ConstraintsReader, error) {
	_, _, _ = catalog, schema, table
	return m.constraintsReader, nil
}
func (m *mockSchemaProvider) RecordsCount(_ context.Context, catalog, schema, table string) (*int, error) {
	_, _, _ = catalog, schema, table
	count := 10
	return &count, nil
}

func TestScanner_ScanCatalog_Bulk(t *testing.T) {
	ctx := context.Background()

	col1 := &datatug.CollectionInfo{
		CollectionKey: datatug.NewCollectionKey(datatug.CollectionTypeTable, "table1", "schema1", "catalog1", nil),
		TableProps: datatug.TableProps{
			DbType: "BASE TABLE",
		},
	}

	mProvider := &mockSchemaProvider{
		isBulk:            true,
		collectionsReader: &mockCollectionsReader{collections: []*datatug.CollectionInfo{col1}},
		columnsReader: &mockColumnsReader{columns: []Column{
			{TableRef: TableRef{SchemaName: "schema1", TableName: "table1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "col1"}}},
			{},
		}},
		indexesReader: &mockIndexesReader{indexes: []*Index{
			{TableRef: TableRef{SchemaName: "schema1", TableName: "table1"}, Index: &datatug.Index{Name: "idx1"}},
		}},
		indexColumnsReader: &mockIndexColumnsReader{columns: []*IndexColumn{
			{TableRef: TableRef{SchemaName: "schema1", TableName: "table1"}, IndexName: "idx1", IndexColumn: &datatug.IndexColumn{Name: "col1"}},
		}},
		constraintsReader: &mockConstraintsReader{constraints: []*Constraint{
			{TableRef: TableRef{SchemaName: "schema1", TableName: "table1"}, ColumnName: "col1", Constraint: &datatug.Constraint{Name: "pk1", Type: "PRIMARY KEY"}},
		}},
	}

	scanner := NewScanner(mProvider)
	catalog, err := scanner.ScanCatalog(ctx, "catalog1")
	if err != nil {
		t.Fatalf("ScanCatalog failed: %v", err)
	}

	if catalog == nil {
		t.Fatal("expected catalog, got nil")
	}
}

type mockCollectionsReader struct {
	collections []*datatug.CollectionInfo
	index       int
}

func (m *mockCollectionsReader) NextCollection() (*datatug.CollectionInfo, error) {
	if m.index >= len(m.collections) {
		return nil, nil
	}
	c := m.collections[m.index]
	m.index++
	return c, nil
}

type mockIndexesReader struct {
	indexes []*Index
	index   int
}

func (m *mockIndexesReader) NextIndex() (*Index, error) {
	if m.index >= len(m.indexes) {
		return &Index{}, nil
	}
	idx := m.indexes[m.index]
	m.index++
	return idx, nil
}

type mockIndexColumnsReader struct {
	columns []*IndexColumn
	index   int
}

func (m *mockIndexColumnsReader) NextIndexColumn() (*IndexColumn, error) {
	if m.index >= len(m.columns) {
		return &IndexColumn{}, nil
	}
	col := m.columns[m.index]
	m.index++
	return col, nil
}

type mockConstraintsReader struct {
	constraints []*Constraint
	index       int
}

func (m *mockConstraintsReader) NextConstraint() (*Constraint, error) {
	if m.index >= len(m.constraints) {
		return &Constraint{}, nil
	}
	c := m.constraints[m.index]
	m.index++
	return c, nil
}

func TestScanner_ScanCatalog(t *testing.T) {
	ctx := context.Background()

	col1 := &datatug.CollectionInfo{
		CollectionKey: datatug.NewCollectionKey(datatug.CollectionTypeTable, "table1", "schema1", "catalog1", nil),
		TableProps: datatug.TableProps{
			DbType: "BASE TABLE",
		},
	}

	mProvider := &mockSchemaProvider{
		collectionsReader: &mockCollectionsReader{collections: []*datatug.CollectionInfo{col1}},
		columnsReader: &mockColumnsReader{columns: []Column{
			{TableRef: TableRef{SchemaName: "schema1", TableName: "table1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "col1"}}},
			{},
		}},
		indexesReader: &mockIndexesReader{indexes: []*Index{
			{TableRef: TableRef{SchemaName: "schema1", TableName: "table1"}, Index: &datatug.Index{Name: "idx1"}},
		}},
		indexColumnsReader: &mockIndexColumnsReader{columns: []*IndexColumn{
			{TableRef: TableRef{SchemaName: "schema1", TableName: "table1"}, IndexName: "idx1", IndexColumn: &datatug.IndexColumn{Name: "col1"}},
		}},
		constraintsReader: &mockConstraintsReader{constraints: []*Constraint{
			{TableRef: TableRef{SchemaName: "schema1", TableName: "table1"}, ColumnName: "col1", Constraint: &datatug.Constraint{Name: "pk1", Type: "PRIMARY KEY"}},
		}},
	}

	scanner := NewScanner(mProvider)
	catalog, err := scanner.ScanCatalog(ctx, "catalog1")
	if err != nil {
		t.Fatalf("ScanCatalog failed: %v", err)
	}

	if catalog == nil {
		t.Fatal("expected catalog, got nil")
	}
}
