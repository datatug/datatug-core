package schemer

import (
	"context"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fkRow is one row of a foreign key as a schema provider reports it: one row
// per column of the key, in the order of the key.
func fkRow(table, name, column, refTable, refColumn string) *Constraint {
	return &Constraint{
		TableRef:        TableRef{SchemaName: "main", TableName: table},
		Constraint:      &datatug.Constraint{Name: name, Type: "FOREIGN KEY"},
		ColumnName:      column,
		RefTableCatalog: "db", RefTableSchema: "main", RefTableName: refTable,
		RefColName: refColumn,
	}
}

// scanFixture scans a catalog of three tables, album, playlist_track and
// track, which a bulk provider reports in that order: its constraints must
// come in the same order.
func scanFixture(t *testing.T, constraints ...*Constraint) []*datatug.CollectionInfo {
	t.Helper()
	collection := func(name string) *datatug.CollectionInfo {
		return &datatug.CollectionInfo{
			DBCollectionKey: datatug.NewTableKey(name, "main", "db", nil),
			TableProps:      datatug.TableProps{DbType: "BASE TABLE"},
		}
	}
	provider := &mockSchemaProvider{
		isBulk:      true,
		collections: []*datatug.CollectionInfo{collection("album"), collection("playlist_track"), collection("track")},
		constraints: constraints,
	}
	catalog, err := NewScanner(provider).ScanCatalog(context.Background(), "db")
	require.NoError(t, err)
	return catalog.Schemas.GetByID("main").Tables
}

func foreignKeysOf(t *testing.T, tables []*datatug.CollectionInfo, name string) datatug.ForeignKeys {
	t.Helper()
	for _, table := range tables {
		if table.Name() == name {
			return table.ForeignKeys
		}
	}
	require.FailNow(t, "table not found", name)
	return nil
}

// TestScan_ForeignKeyKeepsReferencedColumns scans the rows a SQLite
// "PRAGMA foreign_key_list" yields for a composite key and for a single
// column key: the referenced columns are kept, in the order of the key.
func TestScan_ForeignKeyKeepsReferencedColumns(t *testing.T) {
	tables := scanFixture(t,
		// The key lists its columns in an order that is neither the order of
		// the referencing table's columns nor sorted.
		fkRow("playlist_track", "fk_track", "track_b", "track", "id_b"),
		fkRow("playlist_track", "fk_track", "track_a", "track", "id_a"),
		fkRow("track", "fk_album", "album_id", "album", "id"),
	)

	composite := foreignKeysOf(t, tables, "playlist_track")
	require.Len(t, composite, 1)
	assert.Equal(t, []string{"track_b", "track_a"}, composite[0].Columns)
	assert.Equal(t, []string{"id_b", "id_a"}, composite[0].RefColumns)
	assert.NoError(t, composite.Validate())

	single := foreignKeysOf(t, tables, "track")
	require.Len(t, single, 1)
	assert.Equal(t, []string{"id"}, single[0].RefColumns)
}

// TestScan_ForeignKeyFromSourceWithoutReferencedColumns is a source that
// does not report the referenced column: the key stays valid and has none.
func TestScan_ForeignKeyFromSourceWithoutReferencedColumns(t *testing.T) {
	tables := scanFixture(t,
		fkRow("playlist_track", "fk_track", "track_a", "track", ""),
		fkRow("playlist_track", "fk_track", "track_b", "track", ""),
	)
	fks := foreignKeysOf(t, tables, "playlist_track")
	require.Len(t, fks, 1)
	assert.Equal(t, []string{"track_a", "track_b"}, fks[0].Columns)
	assert.Empty(t, fks[0].RefColumns)
	assert.NoError(t, fks.Validate())
}

// TestScan_ForeignKeyWithPartlyReportedReferencedColumns: a source that names
// the referenced column for only some rows of a key cannot be trusted for the
// key, so the key keeps none rather than a list that is out of step with its
// columns, wherever in the key the gap is.
func TestScan_ForeignKeyWithPartlyReportedReferencedColumns(t *testing.T) {
	for name, refColumns := range map[string][3]string{
		"gap_in_the_middle": {"a", "", "c"},
		"gap_at_the_start":  {"", "b", "c"},
		"gap_at_the_end":    {"a", "b", ""},
	} {
		t.Run(name, func(t *testing.T) {
			tables := scanFixture(t,
				fkRow("playlist_track", "fk_track", "c1", "track", refColumns[0]),
				fkRow("playlist_track", "fk_track", "c2", "track", refColumns[1]),
				fkRow("playlist_track", "fk_track", "c3", "track", refColumns[2]),
			)
			fks := foreignKeysOf(t, tables, "playlist_track")
			require.Len(t, fks, 1)
			assert.Equal(t, []string{"c1", "c2", "c3"}, fks[0].Columns)
			assert.Empty(t, fks[0].RefColumns)
			assert.NoError(t, fks.Validate())
		})
	}
}
