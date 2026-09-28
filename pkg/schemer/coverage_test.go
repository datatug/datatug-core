package schemer

import (
	"context"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverage_Deadlines(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
	defer cancel()

	table := &datatug.CollectionInfo{
		DBCollectionKey: datatug.NewTableKey("t", "s", "c", nil),
	}
	tablesFinder := SortedTables{Tables: datatug.Tables{table}}
	idx := &Index{
		TableRef: TableRef{SchemaName: "s", TableName: "t"},
		Index:    &datatug.Index{Name: "idx"},
	}

	provider := &mockSchemaProvider{
		collections: []*datatug.CollectionInfo{table},
		columns: []Column{{
			TableRef:   TableRef{SchemaName: "s", TableName: "t"},
			ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}},
		}},
		indexes:   []*Index{idx},
		indexCols: []*IndexColumn{{TableRef: TableRef{SchemaName: "s", TableName: "t"}, IndexName: "idx"}},
		constraints: []*Constraint{{
			TableRef:   TableRef{SchemaName: "s", TableName: "t"},
			Constraint: &datatug.Constraint{Name: "pk", Type: "PRIMARY KEY"},
			ColumnName: "c1",
		}},
	}
	s := scanner{schemaProvider: provider}

	assert.Error(t, s.scanColumnsInBulk(ctx, "c", tablesFinder))
	assert.Error(t, s.scanIndexesInBulk(ctx, "c", tablesFinder))
	assert.Error(t, s.scanIndexColumnsInBulk(ctx, "c", SortedIndexes{indexes: []*Index{idx}}))
	assert.Error(t, s.scanConstraintsInBulk(ctx, "c", tablesFinder))
	assert.Error(t, s.scanTableCols(ctx, "c", table))
	assert.Error(t, s.scanTableIndexes(ctx, "c", table))
	assert.Error(t, s.scanIndexColumns(ctx, "c", table, idx.Index))
}

func TestCoverage_ScannerNilIndexesAndCols(t *testing.T) {
	table := &datatug.CollectionInfo{
		DBCollectionKey: datatug.NewTableKey("t", "s", "c", nil),
	}
	tablesFinder := SortedTables{Tables: datatug.Tables{table}}

	t.Run("nil_index", func(t *testing.T) {
		provider := &mockSchemaProvider{
			indexes: []*Index{nil},
		}
		s := scanner{schemaProvider: provider}
		err := s.scanIndexesInBulk(context.Background(), "c", tablesFinder)
		require.ErrorContains(t, err, "got nil index at iteration #0")
	})

	t.Run("nil_index_field", func(t *testing.T) {
		provider := &mockSchemaProvider{
			indexes: []*Index{{Index: nil}},
		}
		s := scanner{schemaProvider: provider}
		err := s.scanIndexesInBulk(context.Background(), "c", tablesFinder)
		require.ErrorContains(t, err, "got nil index.Index at iteration #0")
	})

	t.Run("unknown_index_column", func(t *testing.T) {
		provider := &mockSchemaProvider{
			indexCols: []*IndexColumn{
				{
					TableRef:    TableRef{SchemaName: "s", TableName: "t"},
					IndexName:   "missing_idx",
					IndexColumn: &datatug.IndexColumn{Name: "col1"},
				},
			},
		}
		s := scanner{schemaProvider: provider}
		err := s.scanIndexColumnsInBulk(context.Background(), "c", SortedIndexes{})
		require.ErrorContains(t, err, "unknown index referenced by column")
		require.Contains(t, err.Error(), "missing_idx")
	})
}

func TestCoverage_NonBulkTableIndexesAndConstraints(t *testing.T) {
	table := &datatug.CollectionInfo{
		DBCollectionKey: datatug.NewTableKey("t", "s", "c", nil),
	}
	idx := &datatug.Index{Name: "idx1"}

	t.Run("scanTableIndexes_success", func(t *testing.T) {
		provider := &mockSchemaProvider{
			isBulk:      false,
			collections: []*datatug.CollectionInfo{table},
			indexes: []*Index{
				{
					TableRef: TableRef{SchemaName: "s", TableName: "t"},
					Index:    idx,
				},
			},
			indexCols: []*IndexColumn{
				{
					TableRef:    TableRef{SchemaName: "s", TableName: "t"},
					IndexName:   "idx1",
					IndexColumn: &datatug.IndexColumn{Name: "col1"},
				},
			},
		}
		s := scanner{schemaProvider: provider}
		tbl := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("t", "s", "c", nil),
		}
		err := s.scanTableIndexes(context.Background(), "c", tbl)
		require.NoError(t, err)
		require.Len(t, tbl.Indexes, 1)
		require.Len(t, tbl.Indexes[0].Columns, 1)
	})

	t.Run("scanTableConstraints_error", func(t *testing.T) {
		provider := &mockSchemaProvider{
			isBulk: false,
			constraints: []*Constraint{
				{
					TableRef: TableRef{SchemaName: "s", TableName: "t"},
					Constraint: &datatug.Constraint{
						Name: "fk1",
						Type: "FOREIGN KEY",
					},
					ColumnName:      "b_id",
					RefTableCatalog: "c",
					RefTableSchema:  "s",
					RefTableName:    "nonexistent",
				},
			},
		}
		s := scanner{schemaProvider: provider}
		tbl := &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey("t", "s", "c", nil),
		}
		err := s.scanTableConstraints(context.Background(), "c", tbl, datatug.Tables{tbl})
		require.ErrorContains(t, err, "failed to process contraint record")
	})
}

func TestCoverage_ScanConstraints_CompositeKeys(t *testing.T) {
	tableA := &datatug.CollectionInfo{
		DBCollectionKey: datatug.NewTableKey("table_a", "s", "c", nil),
	}
	tableB := &datatug.CollectionInfo{
		DBCollectionKey: datatug.NewTableKey("table_b", "s", "c", nil),
	}
	allTables := datatug.Tables{tableA, tableB}

	// 1. Composite Primary Key
	c1 := &Constraint{
		TableRef:   TableRef{SchemaName: "s", TableName: "table_a"},
		Constraint: &datatug.Constraint{Name: "pk_a", Type: "PRIMARY KEY"},
		ColumnName: "id1",
	}
	c2 := &Constraint{
		TableRef:   TableRef{SchemaName: "s", TableName: "table_a"},
		Constraint: &datatug.Constraint{Name: "pk_a", Type: "PRIMARY KEY"},
		ColumnName: "id2",
	}

	// 2. Composite Foreign Key from Table A to Table B
	fk1 := &Constraint{
		TableRef:        TableRef{SchemaName: "s", TableName: "table_a"},
		Constraint:      &datatug.Constraint{Name: "fk_a_b", Type: "FOREIGN KEY"},
		ColumnName:      "b1",
		RefTableCatalog: "c",
		RefTableSchema:  "s",
		RefTableName:    "table_b",
	}
	fk2 := &Constraint{
		TableRef:        TableRef{SchemaName: "s", TableName: "table_a"},
		Constraint:      &datatug.Constraint{Name: "fk_a_b", Type: "FOREIGN KEY"},
		ColumnName:      "b2",
		RefTableCatalog: "c",
		RefTableSchema:  "s",
		RefTableName:    "table_b",
	}

	require.NoError(t, processConstraint("c", tableA, c1, allTables))
	require.NoError(t, processConstraint("c", tableA, c2, allTables))
	require.Equal(t, []string{"id1", "id2"}, tableA.PrimaryKey.Columns)

	require.NoError(t, processConstraint("c", tableA, fk1, allTables))
	require.NoError(t, processConstraint("c", tableA, fk2, allTables))
	require.Len(t, tableA.ForeignKeys, 1)
	require.Equal(t, []string{"b1", "b2"}, tableA.ForeignKeys[0].Columns)
	require.Len(t, tableB.ReferencedBy, 1)
	require.Len(t, tableB.ReferencedBy[0].ForeignKeys, 1)
}

func TestCoverage_ScanConstraints_InterleavedFK(t *testing.T) {
	tableA := &datatug.CollectionInfo{
		DBCollectionKey: datatug.NewTableKey("table_a", "s", "c", nil),
	}
	tableB := &datatug.CollectionInfo{
		DBCollectionKey: datatug.NewTableKey("table_b", "s", "c", nil),
	}
	allTables := datatug.Tables{tableA, tableB}

	fk1Col1 := &Constraint{
		TableRef:        TableRef{SchemaName: "s", TableName: "table_a"},
		Constraint:      &datatug.Constraint{Name: "fk_1", Type: "FOREIGN KEY"},
		ColumnName:      "col1",
		RefTableCatalog: "c",
		RefTableSchema:  "s",
		RefTableName:    "table_b",
	}
	fk2Col1 := &Constraint{
		TableRef:        TableRef{SchemaName: "s", TableName: "table_a"},
		Constraint:      &datatug.Constraint{Name: "fk_2", Type: "FOREIGN KEY"},
		ColumnName:      "col2",
		RefTableCatalog: "c",
		RefTableSchema:  "s",
		RefTableName:    "table_b",
	}
	fk1Col2 := &Constraint{
		TableRef:        TableRef{SchemaName: "s", TableName: "table_a"},
		Constraint:      &datatug.Constraint{Name: "fk_1", Type: "FOREIGN KEY"},
		ColumnName:      "col3",
		RefTableCatalog: "c",
		RefTableSchema:  "s",
		RefTableName:    "table_b",
	}

	require.NoError(t, processConstraint("c", tableA, fk1Col1, allTables))
	require.NoError(t, processConstraint("c", tableA, fk2Col1, allTables))
	require.NoError(t, processConstraint("c", tableA, fk1Col2, allTables))
}
