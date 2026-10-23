package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/internal/plainfs"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The file is specified in spec/features/model-indexes-file; these tests pin
// that page.

func ixCol(name string) *datatug.IndexColumn { return &datatug.IndexColumn{Name: name} }

func ixDesc(name string) *datatug.IndexColumn {
	return &datatug.IndexColumn{Name: name, IsDescending: true}
}

func ixIncl(name string) *datatug.IndexColumn {
	return &datatug.IndexColumn{Name: name, IsIncludedColumn: true}
}

func ixIndex(name string, columns ...*datatug.IndexColumn) *datatug.Index {
	return &datatug.Index{Name: name, Columns: columns}
}

func ixUnique(index *datatug.Index) *datatug.Index { index.IsUnique = true; return index }

// indexesFixture is the indexes of a small model, in the order the file holds
// them: a single-column index, a composite index with a descending column, a
// unique index, an index with included columns, a composite unique index, a
// partial unique index, a table "a.b" of schema "s" beside a table "b" of
// schema "s.a", and names with a double quote and a backslash.
func indexesFixture() []datatug.TableIndexes {
	partial := ixUnique(ixIndex("ux_orders_open_ref", ixCol("ref")))
	partial.IsPartial = true
	return []datatug.TableIndexes{
		{Table: refsTable("accounting", "ledger"), Indexes: []*datatug.Index{
			ixIndex("ix_ledger_currency", ixCol("currency_code")),
		}},
		{Table: refsTable("public", "invoice"), Indexes: []*datatug.Index{
			ixIndex("ix_invoice_customer_date", ixCol("customer_id"), ixDesc("issued_on")),
			ixUnique(ixIndex("ux_invoice_number", ixCol("number"))),
		}},
		{Table: refsTable("public", "line"), Indexes: []*datatug.Index{
			ixIndex("ix_line_product", ixCol("product_sku"), ixIncl("quantity"), ixIncl("price")),
			ixUnique(ixIndex("pk_line", ixCol("order_id"), ixCol("line_no"))),
		}},
		{Table: refsTable("public", "orders"), Indexes: []*datatug.Index{partial}},
		{Table: refsTable("s", "a.b"), Indexes: []*datatug.Index{
			ixIndex(`ix "quoted" \ back`, ixCol(`c"1`), ixCol(`c\2`)),
		}},
		{Table: refsTable("s.a", "b"), Indexes: []*datatug.Index{
			ixIndex("ix_dot", ixCol("a.b")),
		}},
	}
}

// indexesFixtureShuffled is indexesFixture in another order, with the
// indexes of one table split over two entries and in the other order.
func indexesFixtureShuffled() []datatug.TableIndexes {
	f := indexesFixture()
	return []datatug.TableIndexes{
		f[5],
		{Table: f[2].Table, Indexes: []*datatug.Index{f[2].Indexes[1]}},
		f[4],
		{Table: f[1].Table, Indexes: []*datatug.Index{f[1].Indexes[1], f[1].Indexes[0]}},
		f[3],
		{Table: f[2].Table, Indexes: []*datatug.Index{f[2].Indexes[0]}},
		f[0],
	}
}

const indexesGolden = "testdata/model.indexes.json"

func nestedIndexesPath(root, id string) string {
	return filepath.Join(root, "dbmodels", id, id+".indexes.json")
}

func TestModelIndexes_RoundTripIsPinnedByAGoldenFile(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, SaveModelIndexes(root, "chinook", indexesFixture()))
	assert.Equal(t, string(readBytes(t, indexesGolden)), string(readBytes(t, nestedIndexesPath(root, "chinook"))))

	got, err := LoadModelIndexes(root, "chinook")
	require.NoError(t, err)
	assert.Equal(t, indexesFixture(), got)
}

func TestModelIndexes_SameBytesWhateverTheOrderOfTheInput(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, SaveModelIndexes(root, "m", indexesFixtureShuffled()))
	first := readBytes(t, nestedIndexesPath(root, "m"))
	assert.Equal(t, string(readBytes(t, indexesGolden)), string(first))

	require.NoError(t, SaveModelIndexes(root, "m", indexesFixture()))
	assert.Equal(t, string(first), string(readBytes(t, nestedIndexesPath(root, "m"))), "a second scan of the same database")
}

func TestModelIndexes_WhatIsNotStoredIsDropped(t *testing.T) {
	root := t.TempDir()
	full := &datatug.Index{
		Name: "ix", Type: "BTREE", Origin: "c", Columns: []*datatug.IndexColumn{ixCol("x")},
		IsClustered: true, IsXML: true, IsColumnStore: true, IsHash: true, IsUniqueConstraint: true, IsPrimaryKey: true,
	}
	tables := []datatug.TableIndexes{{Table: datatug.NewTableKey("t", "s", "db", nil), Indexes: []*datatug.Index{nil, full}}}
	require.NoError(t, SaveModelIndexes(root, "m", tables))
	got, err := LoadModelIndexes(root, "m")
	require.NoError(t, err)
	assert.Equal(t, []datatug.TableIndexes{{Table: refsTable("s", "t"), Indexes: []*datatug.Index{ixIndex("ix", ixCol("x"))}}}, got,
		"no type, origin, engine flag or catalog; a nil index is skipped")
}

func TestModelIndexes_ASchemalessTableIsKeptWithoutASchemaMember(t *testing.T) {
	root := t.TempDir()
	tables := []datatug.TableIndexes{{Table: refsTable("", "track"), Indexes: []*datatug.Index{ixIndex("ix", ixCol("album_id"))}}}
	require.NoError(t, SaveModelIndexes(root, "m", tables))
	assert.NotContains(t, string(readBytes(t, nestedIndexesPath(root, "m"))), `"schema"`)
	got, err := LoadModelIndexes(root, "m")
	require.NoError(t, err)
	assert.Equal(t, tables, got)
}

func TestModelIndexes_NoIndexIsAnEmptyListNotAnAbsentOne(t *testing.T) {
	const empty = "{\n\t\"version\": 1,\n\t\"indexes\": []\n}\n"
	root := t.TempDir()
	require.NoError(t, SaveModelIndexes(root, "m", nil))
	assert.Equal(t, empty, string(readBytes(t, nestedIndexesPath(root, "m"))))
	got, err := LoadModelIndexes(root, "m")
	require.NoError(t, err)
	assert.Empty(t, got)

	require.NoError(t, SaveModelIndexes(root, "m", []datatug.TableIndexes{{Table: refsTable("s", "t")}}))
	assert.Equal(t, empty, string(readBytes(t, nestedIndexesPath(root, "m"))))
}

func TestModelIndexes_TheLastSaveHoldsTheModelsIndexes(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, SaveModelIndexes(root, "m", indexesFixture()))
	require.NoError(t, SaveModelIndexes(root, "m", indexesFixture()[:1]))
	got, err := LoadModelIndexes(root, "m")
	require.NoError(t, err)
	assert.Equal(t, indexesFixture()[:1], got)
}

func TestModelIndexes_NoFileIsNoIndexesAndNoError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "dbmodels"), 0o755))
	before := treeHash(t, root)
	got, err := LoadModelIndexes(root, "m")
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Equal(t, before, treeHash(t, root), "a load creates and changes nothing")

	got, err = LoadModelIndexes(filepath.Join(root, "nowhere"), "m")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestModelIndexes_Location(t *testing.T) {
	tables := indexesFixture()[:1]

	t.Run("nested_model", func(t *testing.T) {
		root := t.TempDir()
		writeRaw(t, filepath.Join(root, "dbmodels", "m", "m.dbmodel.json"), `{"id":"m"}`)
		require.NoError(t, SaveModelIndexes(root, "m", tables))
		assert.FileExists(t, filepath.Join(root, "dbmodels", "m", "m.indexes.json"))
		assert.NoFileExists(t, filepath.Join(root, "dbmodels", "m.indexes.json"))
	})

	t.Run("flat_model", func(t *testing.T) {
		root := t.TempDir()
		writeRaw(t, filepath.Join(root, "dbmodels", "m.dbmodel.json"), `{"id":"m"}`)
		require.NoError(t, SaveModelIndexes(root, "m", tables))
		assert.FileExists(t, filepath.Join(root, "dbmodels", "m.indexes.json"))
		assert.NoDirExists(t, filepath.Join(root, "dbmodels", "m"))
		got, err := LoadModelIndexes(root, "m")
		require.NoError(t, err)
		assert.Equal(t, tables, got)
	})

	t.Run("model_without_a_file_of_its_own_gets_the_folder_a_new_model_gets", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, SaveModelIndexes(root, "m", tables))
		assert.FileExists(t, nestedIndexesPath(root, "m"))
	})

	t.Run("a_file_in_the_other_folder_is_not_read", func(t *testing.T) {
		root := t.TempDir()
		writeRaw(t, filepath.Join(root, "dbmodels", "m", "m.dbmodel.json"), `{"id":"m"}`)
		writeRaw(t, filepath.Join(root, "dbmodels", "m.indexes.json"), "not even json")
		got, err := LoadModelIndexes(root, "m")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("beside_the_foreign_keys_file", func(t *testing.T) {
		for _, layout := range []string{"nested", "flat", "none"} {
			root := t.TempDir()
			switch layout {
			case "nested":
				writeRaw(t, filepath.Join(root, "dbmodels", "m", "m.dbmodel.json"), `{"id":"m"}`)
			case "flat":
				writeRaw(t, filepath.Join(root, "dbmodels", "m.dbmodel.json"), `{"id":"m"}`)
			}
			store := newFsDbModelsStore(root)
			refsRel, _ := store.foreignKeysFile("m")
			rel, _ := store.indexesFile("m")
			assert.Equal(t, strings.TrimSuffix(refsRel, ".refs.json")+".indexes.json", rel, layout)
		}
	})
}

func TestModelIndexes_ASaveChangesNoOtherFileAndTheModelsStillLoad(t *testing.T) {
	root := t.TempDir()
	ownFile := filepath.Join(root, "dbmodels", "m", "m.dbmodel.json")
	writeRaw(t, ownFile, `{"id":"m","title":"Model"}`)
	require.NoError(t, SaveModelForeignKeys(root, "m", refsFixture()))
	refsBefore := readBytes(t, nestedRefsPath(root, "m"))
	ownBefore := readBytes(t, ownFile)

	require.NoError(t, SaveModelIndexes(root, "m", indexesFixture()))

	assert.Equal(t, string(ownBefore), string(readBytes(t, ownFile)))
	assert.Equal(t, string(refsBefore), string(readBytes(t, nestedRefsPath(root, "m"))))
	store := newFsDbModelsStore(root)
	ids, err := store.listDbModelIDs()
	require.NoError(t, err)
	assert.Equal(t, []string{"m"}, ids, "the file is not taken for a model")
	models, err := store.LoadDbModels(t.Context())
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "Model", models[0].Title)
}

func TestModelIndexes_UnknownMembersAreIgnoredAndNullIsAbsent(t *testing.T) {
	root := t.TempDir()
	writeRaw(t, nestedIndexesPath(root, "m"), `{
		"version": 1,
		"generatedBy": "a later release",
		"indexes": [
			{
				"name": "ix",
				"method": "gin",
				"table": {"schema": null, "name": "a", "catalog": "db"},
				"columns": [{"name": "x", "collation": "C"}, {"name": "y", "descending": null}],
				"unique": null,
				"where": "y > 0"
			}
		]
	}`)
	got, err := LoadModelIndexes(root, "m")
	require.NoError(t, err)
	assert.Equal(t, []datatug.TableIndexes{{Table: refsTable("", "a"), Indexes: []*datatug.Index{
		ixIndex("ix", ixCol("x"), ixCol("y")),
	}}}, got)
}

func TestModelIndexes_LoadReturnsTheCanonicalOrderAndKeepsColumnOrder(t *testing.T) {
	root := t.TempDir()
	writeRaw(t, nestedIndexesPath(root, "m"), `{"version":1,"indexes":[
		{"name":"b","table":{"name":"t"},"columns":[{"name":"x"}]},
		{"name":"a","table":{"name":"t"},"columns":[{"name":"y"},{"name":"x"}]}
	]}`)
	got, err := LoadModelIndexes(root, "m")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].Indexes, 2)
	assert.Equal(t, ixIndex("a", ixCol("y"), ixCol("x")), got[0].Indexes[0], "the columns keep the index's order")
}

func TestModelIndexes_LoadRefusals(t *testing.T) {
	const file = "dbmodels/m/m.indexes.json"
	const one = `{"name":"ix","table":{"name":"a"},"columns":[{"name":"x"},{"name":"y"}]}`
	entry := func() string { return `{"version":1,"indexes":[` + one + `]}` }
	cases := map[string]struct{ content, want string }{
		"not_json":          {"this is not json", "is not a JSON object of the expected form"},
		"json_array":        {"[1]", "is not a JSON object of the expected form"},
		"not_utf8":          {"{\"version\":1,\"indexes\":[{\"name\":\"i\xff\"}]}", "is not valid UTF-8 text"},
		"indexes_not_list":  {`{"version":1,"indexes":{}}`, "is not a JSON object of the expected form"},
		"unique_not_bool":   {strings.Replace(entry(), `"columns"`, `"unique":"yes","columns"`, 1), "is not a JSON object of the expected form"},
		"no_indexes_member": {`{"version":1}`, `has no "indexes" list`},
		"indexes_null":      {`{"version":1,"indexes":null}`, `has no "indexes" list`},
		"no_version":        {`{"indexes":[]}`, `has no "version"`},
		"unknown_version":   {`{"version":2,"indexes":[]}`, "is version 2, which this release does not know (it reads version 1)"},
		"no_name":           {strings.Replace(entry(), `"name":"ix",`, "", 1), `index at index 0 of table "a" has no name`},
		"no_table_name":     {strings.Replace(entry(), `"table":{"name":"a"}`, `"table":{}`, 1), "has no table name"},
		"no_columns":        {strings.Replace(entry(), `[{"name":"x"},{"name":"y"}]`, `[]`, 1), "has no columns"},
		"columns_absent":    {strings.Replace(entry(), `,"columns":[{"name":"x"},{"name":"y"}]`, ``, 1), "has no columns"},
		"empty_column_name": {strings.Replace(entry(), `{"name":"y"}`, `{"name":""}`, 1), "has an empty column name"},
		"only_included": {strings.Replace(entry(), `[{"name":"x"},{"name":"y"}]`, `[{"name":"x","included":true}]`, 1),
			"has only included columns and no key column"},
		"duplicate": {`{"version":1,"indexes":[` + one + `,` + one + `]}`, `index "ix" of table "a" is there twice`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeRaw(t, filepath.Join(root, filepath.FromSlash(file)), c.content)
			_, err := LoadModelIndexes(root, "m")
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
			assert.Contains(t, err.Error(), file)
			assert.NotContains(t, err.Error(), root)
		})
	}
}

func TestModelIndexes_LoadRefusesAFileThatIsNotAPlainFile(t *testing.T) {
	t.Run("folder", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(nestedIndexesPath(root, "m"), 0o755))
		_, err := LoadModelIndexes(root, "m")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dbmodels/m/m.indexes.json")
		assert.Contains(t, err.Error(), "not a regular file")
		assert.NotContains(t, err.Error(), root)
		var entryErr *nonRegularEntryError
		assert.True(t, errors.As(err, &entryErr))
	})

	t.Run("link", func(t *testing.T) {
		skipSymlinksOnWindows(t)
		root := t.TempDir()
		outside := filepath.Join(t.TempDir(), "elsewhere.json")
		writeRaw(t, outside, `{"version":1,"indexes":[]}`)
		require.NoError(t, os.MkdirAll(filepath.Dir(nestedIndexesPath(root, "m")), 0o755))
		require.NoError(t, os.Symlink(outside, nestedIndexesPath(root, "m")))
		_, err := LoadModelIndexes(root, "m")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dbmodels/m/m.indexes.json")
		assert.Contains(t, err.Error(), "symlink")
		assert.NotContains(t, err.Error(), root)
		assert.NotContains(t, err.Error(), outside)
	})

	t.Run("over_the_size_limit", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Dir(nestedIndexesPath(root, "m")), 0o755))
		f, err := os.Create(nestedIndexesPath(root, "m"))
		require.NoError(t, err)
		require.NoError(t, f.Truncate(maxIndexesFileSize+1))
		require.NoError(t, f.Close())
		_, err = LoadModelIndexes(root, "m")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dbmodels/m/m.indexes.json")
		assert.NotContains(t, err.Error(), root)
	})
}

func TestModelIndexes_ModelIdThatNamesNoFile(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"", ".", "..", "a/b", `a\b`, "../outside"} {
		t.Run(id, func(t *testing.T) {
			_, err := LoadModelIndexes(root, id)
			assert.ErrorContains(t, err, "cannot name an indexes file")
			err = SaveModelIndexes(root, id, indexesFixture())
			assert.ErrorContains(t, err, "cannot name an indexes file")
		})
	}
	assert.Equal(t, 0, len(mustReadDir(t, root)))
}

// A writer refuses before it touches the file.
func TestModelIndexes_SaveRefusals(t *testing.T) {
	bad := "a\xffb"
	index := func(mutate func(*datatug.Index)) []datatug.TableIndexes {
		ix := ixIndex("ix", ixCol("x"), ixCol("y"))
		mutate(ix)
		return []datatug.TableIndexes{{Table: refsTable("s", "a"), Indexes: []*datatug.Index{ix}}}
	}
	cases := map[string]struct {
		tables []datatug.TableIndexes
		want   string
	}{
		"no_name":         {index(func(ix *datatug.Index) { ix.Name = "" }), "has no name"},
		"no_columns":      {index(func(ix *datatug.Index) { ix.Columns = nil }), "has no columns"},
		"empty_column":    {index(func(ix *datatug.Index) { ix.Columns[1].Name = "" }), "an index on an expression cannot be stored"},
		"nil_column":      {index(func(ix *datatug.Index) { ix.Columns[0] = nil }), "has an empty column name"},
		"only_included":   {index(func(ix *datatug.Index) { ix.Columns = []*datatug.IndexColumn{ixIncl("x")} }), "no key column"},
		"name_not_utf8":   {index(func(ix *datatug.Index) { ix.Name = bad }), "not valid UTF-8 text"},
		"column_not_utf8": {index(func(ix *datatug.Index) { ix.Columns[0].Name = bad }), "not valid UTF-8 text"},
		"schema_not_utf8": {[]datatug.TableIndexes{{Table: refsTable(bad, "a"), Indexes: []*datatug.Index{ixIndex("ix", ixCol("x"))}}},
			"not valid UTF-8 text"},
		"table_not_utf8": {[]datatug.TableIndexes{{Table: refsTable("s", bad), Indexes: []*datatug.Index{ixIndex("ix", ixCol("x"))}}},
			"not valid UTF-8 text"},
		"table_has_no_name": {[]datatug.TableIndexes{{Table: datatug.DBCollectionKey{}, Indexes: []*datatug.Index{ixIndex("ix", ixCol("x"))}}},
			"has no table name"},
		"duplicate_in_two_entries": {[]datatug.TableIndexes{
			{Table: refsTable("s", "a"), Indexes: []*datatug.Index{ixIndex("ix", ixCol("x"))}},
			{Table: refsTable("s", "a"), Indexes: []*datatug.Index{ixIndex("ix", ixCol("y"))}},
		}, `index "ix" of table "s"."a" is there twice`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			err := SaveModelIndexes(root, "m", c.tables)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
			assert.Contains(t, err.Error(), "dbmodels/m/m.indexes.json")
			assert.Equal(t, 0, len(mustReadDir(t, root)), "nothing was written")
		})
	}

	t.Run("a_refusal_keeps_the_file_of_an_earlier_save", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, SaveModelIndexes(root, "m", indexesFixture()))
		before := readBytes(t, nestedIndexesPath(root, "m"))
		require.Error(t, SaveModelIndexes(root, "m", index(func(ix *datatug.Index) { ix.Name = "" })))
		assert.Equal(t, string(before), string(readBytes(t, nestedIndexesPath(root, "m"))))
	})
}

func TestModelIndexes_SaveRefusesALinkAndAFolderInTheFilesPlace(t *testing.T) {
	const file = "dbmodels/m/m.indexes.json"

	t.Run("live_link", func(t *testing.T) {
		f := newLinkFixture(t)
		f.mkdirsAbove(t, file)
		f.link(t, filepath.Join(f.outside, "keep.txt"), file)
		before := treeHash(t, f.outside)
		err := SaveModelIndexes(f.root, "m", indexesFixture())
		require.ErrorIs(t, err, plainfs.ErrNotPlain)
		f.requireRefusedAndOutsideIntact(t, err, before, false)
		assert.Contains(t, err.Error(), file)
	})

	t.Run("flat_link", func(t *testing.T) {
		f := newLinkFixture(t)
		writeRaw(t, filepath.Join(f.root, "dbmodels", "m.dbmodel.json"), `{"id":"m"}`)
		f.link(t, filepath.Join(f.outside, "keep.txt"), "dbmodels/m.indexes.json")
		before := treeHash(t, f.outside)
		err := SaveModelIndexes(f.root, "m", indexesFixture())
		require.ErrorIs(t, err, plainfs.ErrNotPlain)
		f.requireRefusedAndOutsideIntact(t, err, before, false)
		assert.Contains(t, err.Error(), "dbmodels/m.indexes.json")
	})

	t.Run("folder", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(nestedIndexesPath(root, "m"), 0o755))
		err := SaveModelIndexes(root, "m", indexesFixture())
		require.ErrorIs(t, err, plainfs.ErrNotPlain)
		assert.Contains(t, err.Error(), file)
	})
}

// A name reaches an error text quoted, so a newline or a terminal escape in
// it cannot pass for something else.
func TestModelIndexes_RefusalsNameTheTableQuoted(t *testing.T) {
	tricky := "evil\n\x1b[31mtable"
	tables := []datatug.TableIndexes{{Table: refsTable("s.a", tricky), Indexes: []*datatug.Index{
		ixIndex("ix", ixCol("x")), ixIndex("ix", ixCol("y")),
	}}}
	err := SaveModelIndexes(t.TempDir(), "m", tables)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `index "ix" of table "s.a"."evil\n\x1b[31mtable" is there twice`)
	assert.NotContains(t, err.Error(), tricky)
}

// The page says how a string is written; the expected text is spelled out here
// from the page, not taken from the encoder.
func TestModelIndexes_StringsAreWrittenAsThePageSays(t *testing.T) {
	name := "q\" b\\ lf\n tab\t nul\x00 del\x7f ls\u2028 <>& é"
	root := t.TempDir()
	tables := []datatug.TableIndexes{{Table: refsTable("", "t"), Indexes: []*datatug.Index{ixIndex(name, ixCol("x"))}}}
	require.NoError(t, SaveModelIndexes(root, "m", tables))
	want := `{
	"version": 1,
	"indexes": [
		{
			"name": "q\" b\\ lf\n tab\t nul\u0000 del` + "\x7f" + ` ls\u2028 <>& é",
			"table": {
				"name": "t"
			},
			"columns": [
				{
					"name": "x"
				}
			]
		}
	]
}
`
	assert.Equal(t, want, string(readBytes(t, nestedIndexesPath(root, "m"))))
	got, err := LoadModelIndexes(root, "m")
	require.NoError(t, err)
	assert.Equal(t, tables, got)
}

// Deleting a model removes the file of its indexes with its own file, in each
// layout.
func TestModelIndexes_DeletingTheModelRemovesTheFile(t *testing.T) {
	ctx := context.Background()
	for _, layout := range []string{"nested", "flat"} {
		t.Run(layout, func(t *testing.T) {
			root := t.TempDir()
			own := filepath.Join(root, "dbmodels", "m", "m.dbmodel.json")
			if layout == "flat" {
				own = filepath.Join(root, "dbmodels", "m.dbmodel.json")
			}
			writeRaw(t, own, `{"id":"m"}`)
			require.NoError(t, SaveModelIndexes(root, "m", indexesFixture()))
			require.NoError(t, SaveModelIndexes(root, "other", indexesFixture()))

			require.NoError(t, newFsDbModelsStore(root).DeleteDbModel(ctx, "m"))

			assert.NoFileExists(t, filepath.Join(root, "dbmodels", "m.indexes.json"))
			assert.NoFileExists(t, nestedIndexesPath(root, "m"))
			assert.FileExists(t, nestedIndexesPath(root, "other"), "another model's file stays")
		})
	}

	t.Run("a_refusal_on_the_indexes_file_leaves_the_model_in_place", func(t *testing.T) {
		skipSymlinksOnWindows(t)
		root := t.TempDir()
		model := filepath.Join(root, "dbmodels", "m", "m.dbmodel.json")
		writeRaw(t, model, `{"id":"m"}`)
		require.NoError(t, os.Symlink(t.TempDir(), nestedIndexesPath(root, "m")))
		require.Error(t, newFsDbModelsStore(root).DeleteDbModel(ctx, "m"))
		assert.FileExists(t, model)
	})
}
