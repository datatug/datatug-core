package schemer

import (
	"context"
	"io"

	"github.com/dal-go/dalgo/dal"
	"github.com/datatug/datatug-core/pkg/datatug"
)

type mockSchemaProvider struct {
	isBulk             bool
	collections        []*datatug.CollectionInfo
	columns            []Column
	indexes            []*Index
	indexCols          []*IndexColumn
	constraints        []*Constraint
	recordsCount       map[string]int
	err                error
	getCollectionsErr  error
	getColumnsErr      error
	getIndexesErr      error
	getIndexColumnsErr error
	getConstraintsErr  error
	recordsCountErr    error
	nextCollectionErr  error
	nextColumnErr      error
	nextIndexErr       error
	nextIndexColumnErr error
	nextConstraintErr  error
}

func (m *mockSchemaProvider) IsBulkProvider() bool { return m.isBulk }

func (m *mockSchemaProvider) GetCollections(_ context.Context, _ *dal.Key) (CollectionsReader, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.getCollectionsErr != nil {
		return nil, m.getCollectionsErr
	}
	return &mockCollectionsReader{collections: m.collections, err: m.nextCollectionErr}, nil
}

type mockCollectionsReader struct {
	collections []*datatug.CollectionInfo
	index       int
	err         error
}

func (m *mockCollectionsReader) NextCollection() (*datatug.CollectionInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.index >= len(m.collections) {
		return nil, io.EOF
	}
	c := m.collections[m.index]
	m.index++
	return c, nil
}

type mockColumnsReader struct {
	columns []Column
	index   int
	err     error
}

func (m *mockColumnsReader) NextColumn() (Column, error) {
	if m.err != nil {
		return Column{}, m.err
	}
	if m.index >= len(m.columns) {
		return Column{}, io.EOF
	}
	col := m.columns[m.index]
	m.index++
	return col, nil
}

func (m *mockSchemaProvider) GetColumnsReader(c context.Context, catalog string, filter ColumnsFilter) (ColumnsReader, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.getColumnsErr != nil {
		return nil, m.getColumnsErr
	}
	return &mockColumnsReader{columns: m.columns, err: m.nextColumnErr}, nil
}

func (m *mockSchemaProvider) GetColumns(c context.Context, catalog string, filter ColumnsFilter) ([]Column, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.columns, nil
}

func (m *mockSchemaProvider) GetIndexes(c context.Context, catalog, schema, table string) (IndexesReader, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.getIndexesErr != nil {
		return nil, m.getIndexesErr
	}
	return &mockIndexesReader{indexes: m.indexes, err: m.nextIndexErr}, nil
}

type mockIndexesReader struct {
	indexes []*Index
	index   int
	err     error
}

func (m *mockIndexesReader) NextIndex() (*Index, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.index >= len(m.indexes) {
		return nil, io.EOF
	}
	idx := m.indexes[m.index]
	m.index++
	return idx, nil
}

func (m *mockSchemaProvider) GetIndexColumns(c context.Context, catalog, schema, table, index string) (IndexColumnsReader, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.getIndexColumnsErr != nil {
		return nil, m.getIndexColumnsErr
	}
	return &mockIndexColumnsReader{indexCols: m.indexCols, err: m.nextIndexColumnErr}, nil
}

type mockIndexColumnsReader struct {
	indexCols []*IndexColumn
	index     int
	err       error
}

func (m *mockIndexColumnsReader) NextIndexColumn() (*IndexColumn, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.index >= len(m.indexCols) {
		return nil, io.EOF
	}
	ic := m.indexCols[m.index]
	m.index++
	return ic, nil
}

func (m *mockSchemaProvider) GetConstraints(c context.Context, catalog, schema, table string) (ConstraintsReader, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.getConstraintsErr != nil {
		return nil, m.getConstraintsErr
	}
	return &mockConstraintsReader{constraints: m.constraints, err: m.nextConstraintErr}, nil
}

type mockConstraintsReader struct {
	constraints []*Constraint
	index       int
	err         error
}

func (m *mockConstraintsReader) NextConstraint() (*Constraint, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.index >= len(m.constraints) {
		return nil, io.EOF
	}
	cs := m.constraints[m.index]
	m.index++
	return cs, nil
}

func (m *mockSchemaProvider) RecordsCount(c context.Context, catalog, schema, table string) (*int, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.recordsCountErr != nil {
		return nil, m.recordsCountErr
	}
	count, ok := m.recordsCount[catalog+"."+schema+"."+table]
	if !ok {
		return nil, nil
	}
	return &count, nil
}

func (m *mockSchemaProvider) GetReferrers(c context.Context, schema, table string) ([]ForeignKey, error) {
	return nil, nil
}

func (m *mockSchemaProvider) GetForeignKeysReader(c context.Context, schema, table string) (ForeignKeysReader, error) {
	return nil, nil
}

func (m *mockSchemaProvider) GetForeignKeys(c context.Context, schema, table string) ([]ForeignKey, error) {
	return nil, nil
}
