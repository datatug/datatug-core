package filestore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/internal/plainfs"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The file is specified in spec/features/model-foreign-keys-file; these tests
// pin that page.

func refsTable(schema, name string) datatug.DBCollectionKey {
	return datatug.NewTableKey(name, schema, "", nil)
}

func refsKey(name string, columns []string, refTable datatug.DBCollectionKey, refColumns ...string) *datatug.ForeignKey {
	return &datatug.ForeignKey{Name: name, Columns: columns, RefTable: refTable, RefColumns: refColumns}
}

// refsFixture is the foreign keys of a small model, in the order the file
// holds them: a single-column key, a composite key whose columns are not in
// the order of the referenced table's own, a key to a table of another
// schema, a key from a table to itself, two keys between the same two tables,
// and a table of a schema that sorts first. Names keep the spelling the database gives them, including case,
// a space and a character that a JSON encoder escapes by default.
func refsFixture() []datatug.TableForeignKeys {
	return []datatug.TableForeignKeys{
		{Table: refsTable("accounting", "ledger"), ForeignKeys: datatug.ForeignKeys{
			refsKey("fk_ledger_currency", []string{"currency_code"}, refsTable("reference", "currency"), "code"),
		}},
		{Table: refsTable("public", "Employee"), ForeignKeys: datatug.ForeignKeys{
			refsKey("fk_employee_manager", []string{"manager_id"}, refsTable("public", "Employee"), "id"),
		}},
		{Table: refsTable("public", "R&D <notes>"), ForeignKeys: datatug.ForeignKeys{
			refsKey("fk notes", []string{"Ünïcode col"}, refsTable("public", "orders"), "id"),
		}},
		{Table: refsTable("public", "invoice"), ForeignKeys: datatug.ForeignKeys{
			refsKey("fk_invoice_ledger", []string{"ledger_id"}, refsTable("accounting", "ledger"), "id"),
		}},
		{Table: refsTable("public", "line"), ForeignKeys: datatug.ForeignKeys{
			refsKey("fk_line_order", []string{"order_id"}, refsTable("public", "orders"), "id"),
			refsKey("fk_line_product", []string{"product_vendor", "product_sku"}, refsTable("public", "product"), "vendor", "sku"),
		}},
		{Table: refsTable("public", "orders"), ForeignKeys: datatug.ForeignKeys{
			refsKey("fk_orders_billing", []string{"billing_address_id"}, refsTable("public", "address"), "id"),
			refsKey("fk_orders_shipping", []string{"shipping_address_id"}, refsTable("public", "address"), "id"),
		}},
	}
}

// refsFixtureShuffled is refsFixture in another order, with the keys of two
// tables split over two entries and in the other order.
func refsFixtureShuffled() []datatug.TableForeignKeys {
	f := refsFixture()
	return []datatug.TableForeignKeys{
		f[2],
		{Table: f[5].Table, ForeignKeys: datatug.ForeignKeys{f[5].ForeignKeys[1]}},
		f[3],
		{Table: f[4].Table, ForeignKeys: datatug.ForeignKeys{f[4].ForeignKeys[1], f[4].ForeignKeys[0]}},
		{Table: f[5].Table, ForeignKeys: datatug.ForeignKeys{f[5].ForeignKeys[0]}},
		f[1],
		f[0],
	}
}

const refsGolden = "testdata/model.refs.json"

func readBytes(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return b
}

func writeRaw(t *testing.T, p, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func nestedRefsPath(root, id string) string {
	return filepath.Join(root, "dbmodels", id, id+".refs.json")
}

func TestModelForeignKeys_RoundTripIsPinnedByAGoldenFile(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, SaveModelForeignKeys(root, "chinook", refsFixture()))

	assert.Equal(t, string(readBytes(t, refsGolden)), string(readBytes(t, nestedRefsPath(root, "chinook"))))

	got, err := LoadModelForeignKeys(root, "chinook")
	require.NoError(t, err)
	assert.Equal(t, refsFixture(), got)
}

func TestModelForeignKeys_SameBytesWhateverTheOrderOfTheInput(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, SaveModelForeignKeys(root, "m", refsFixtureShuffled()))
	first := readBytes(t, nestedRefsPath(root, "m"))
	assert.Equal(t, string(readBytes(t, refsGolden)), string(first))

	require.NoError(t, SaveModelForeignKeys(root, "m", refsFixture()))
	assert.Equal(t, string(first), string(readBytes(t, nestedRefsPath(root, "m"))), "a second scan of the same database")

	got, err := LoadModelForeignKeys(root, "m")
	require.NoError(t, err)
	assert.Equal(t, refsFixture(), got)
}

func TestModelForeignKeys_ASchemalessTableIsKeptWithoutASchemaMember(t *testing.T) {
	root := t.TempDir()
	keys := []datatug.TableForeignKeys{{Table: refsTable("", "track"), ForeignKeys: datatug.ForeignKeys{
		refsKey("fk_album", []string{"album_id"}, refsTable("", "album"), "id"),
	}}}
	require.NoError(t, SaveModelForeignKeys(root, "m", keys))
	assert.NotContains(t, string(readBytes(t, nestedRefsPath(root, "m"))), `"schema"`)
	got, err := LoadModelForeignKeys(root, "m")
	require.NoError(t, err)
	assert.Equal(t, keys, got)
}

func TestModelForeignKeys_AKeyTheCatalogOfWhichIsTheSameOrOneIsUnknownIsStored(t *testing.T) {
	root := t.TempDir()
	in := func(catalog, name string) datatug.DBCollectionKey {
		return datatug.NewTableKey(name, "s", catalog, nil)
	}
	keys := []datatug.TableForeignKeys{
		{Table: in("db", "a"), ForeignKeys: datatug.ForeignKeys{refsKey("fk1", []string{"x"}, in("db", "b"), "id")}},
		{Table: in("db", "c"), ForeignKeys: datatug.ForeignKeys{refsKey("fk2", []string{"x"}, in("", "b"), "id")}},
		{Table: in("", "d"), ForeignKeys: datatug.ForeignKeys{refsKey("fk3", []string{"x"}, in("db", "b"), "id")}},
	}
	require.NoError(t, SaveModelForeignKeys(root, "m", keys))
	got, err := LoadModelForeignKeys(root, "m")
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "a", got[0].Table.Name())
	assert.Equal(t, "", got[0].Table.Catalog(), "the catalog is not stored")
}

func TestModelForeignKeys_NoKeyIsAnEmptyListNotAnAbsentOne(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, SaveModelForeignKeys(root, "m", nil))
	assert.Equal(t, "{\n\t\"version\": 1,\n\t\"foreignKeys\": []\n}\n", string(readBytes(t, nestedRefsPath(root, "m"))))

	got, err := LoadModelForeignKeys(root, "m")
	require.NoError(t, err)
	assert.Empty(t, got)

	// A table without a key adds nothing.
	require.NoError(t, SaveModelForeignKeys(root, "m", []datatug.TableForeignKeys{{Table: refsTable("s", "t")}}))
	assert.Equal(t, "{\n\t\"version\": 1,\n\t\"foreignKeys\": []\n}\n", string(readBytes(t, nestedRefsPath(root, "m"))))
}

func TestModelForeignKeys_ASaveReplacesTheKeysOfAnEarlierSave(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, SaveModelForeignKeys(root, "m", refsFixture()))
	require.NoError(t, SaveModelForeignKeys(root, "m", refsFixture()[:1]))
	got, err := LoadModelForeignKeys(root, "m")
	require.NoError(t, err)
	assert.Equal(t, refsFixture()[:1], got)
}

func TestModelForeignKeys_NoFileIsNoKeysAndNoError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "dbmodels"), 0o755))
	before := treeHash(t, root)

	for _, id := range []string{"m", "with.dots"} {
		got, err := LoadModelForeignKeys(root, id)
		require.NoError(t, err)
		assert.Empty(t, got)
	}
	assert.Equal(t, before, treeHash(t, root), "a load creates and changes nothing")

	// Not even a project folder: still no keys.
	got, err := LoadModelForeignKeys(filepath.Join(root, "nowhere"), "m")
	require.NoError(t, err)
	assert.Empty(t, got)
}

// The file is beside the model's own file, found the way the model's other
// files are found.
func TestModelForeignKeys_Location(t *testing.T) {
	keys := refsFixture()[:1]

	t.Run("nested_model", func(t *testing.T) {
		root := t.TempDir()
		writeRaw(t, filepath.Join(root, "dbmodels", "m", "m.dbmodel.json"), `{"id":"m"}`)
		require.NoError(t, SaveModelForeignKeys(root, "m", keys))
		assert.FileExists(t, filepath.Join(root, "dbmodels", "m", "m.refs.json"))
		assert.NoFileExists(t, filepath.Join(root, "dbmodels", "m.refs.json"))
	})

	t.Run("flat_model", func(t *testing.T) {
		root := t.TempDir()
		writeRaw(t, filepath.Join(root, "dbmodels", "m.dbmodel.json"), `{"id":"m"}`)
		require.NoError(t, SaveModelForeignKeys(root, "m", keys))
		assert.FileExists(t, filepath.Join(root, "dbmodels", "m.refs.json"))
		assert.NoDirExists(t, filepath.Join(root, "dbmodels", "m"))

		got, err := LoadModelForeignKeys(root, "m")
		require.NoError(t, err)
		assert.Equal(t, keys, got)
	})

	t.Run("both_layouts_nested_wins", func(t *testing.T) {
		root := t.TempDir()
		writeRaw(t, filepath.Join(root, "dbmodels", "m", "m.dbmodel.json"), `{"id":"m"}`)
		writeRaw(t, filepath.Join(root, "dbmodels", "m.dbmodel.json"), `{"id":"m"}`)
		require.NoError(t, SaveModelForeignKeys(root, "m", keys))
		assert.FileExists(t, filepath.Join(root, "dbmodels", "m", "m.refs.json"))
	})

	t.Run("model_without_a_file_of_its_own_gets_the_folder_a_new_model_gets", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, SaveModelForeignKeys(root, "m", keys))
		assert.FileExists(t, filepath.Join(root, "dbmodels", "m", "m.refs.json"))
	})

	t.Run("a_file_in_the_other_folder_is_not_read", func(t *testing.T) {
		root := t.TempDir()
		writeRaw(t, filepath.Join(root, "dbmodels", "m", "m.dbmodel.json"), `{"id":"m"}`)
		writeRaw(t, filepath.Join(root, "dbmodels", "m.refs.json"), "not even json")
		got, err := LoadModelForeignKeys(root, "m")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("same_folder_as_the_model_store_saves_the_model_in", func(t *testing.T) {
		for _, layout := range []string{"nested", "flat", "none"} {
			root := t.TempDir()
			switch layout {
			case "nested":
				writeRaw(t, filepath.Join(root, "dbmodels", "m", "m.dbmodel.json"), `{"id":"m"}`)
			case "flat":
				writeRaw(t, filepath.Join(root, "dbmodels", "m.dbmodel.json"), `{"id":"m"}`)
			}
			store := newFsDbModelsStore(root)
			dir, _ := store.saveTarget("m")
			rel, full := store.foreignKeysFile("m")
			assert.Equal(t, filepath.Join(dir, "m.refs.json"), filepath.FromSlash(full), layout)
			assert.True(t, strings.HasPrefix(rel, "dbmodels/"), rel)
		}
	})
}

func TestModelForeignKeys_ASaveChangesNoOtherFileAndTheModelsStillLoad(t *testing.T) {
	for _, layout := range []string{"nested", "flat"} {
		t.Run(layout, func(t *testing.T) {
			root := t.TempDir()
			ownFile := filepath.Join(root, "dbmodels", "m", "m.dbmodel.json")
			if layout == "flat" {
				ownFile = filepath.Join(root, "dbmodels", "m.dbmodel.json")
			}
			writeRaw(t, ownFile, `{"id":"m","title":"Model"}`)
			writeRaw(t, filepath.Join(root, "datatug-project.json"), `{"id":"p"}`)
			before := treeHash(t, root)
			ownBefore := readBytes(t, ownFile)

			require.NoError(t, SaveModelForeignKeys(root, "m", refsFixture()))

			assert.Equal(t, string(ownBefore), string(readBytes(t, ownFile)))
			assert.Equal(t, `{"id":"p"}`, string(readBytes(t, filepath.Join(root, "datatug-project.json"))))
			assert.NotEqual(t, before, treeHash(t, root), "the new file is there")

			store := newFsDbModelsStore(root)
			ids, err := store.listDbModelIDs()
			require.NoError(t, err)
			assert.Equal(t, []string{"m"}, ids, "the file is not taken for a model")
			models, err := store.LoadDbModels(t.Context())
			require.NoError(t, err)
			require.Len(t, models, 1)
			assert.Equal(t, "Model", models[0].Title)
		})
	}
}

func TestModelForeignKeys_UnknownMembersAreIgnored(t *testing.T) {
	root := t.TempDir()
	writeRaw(t, nestedRefsPath(root, "m"), `{
		"version": 1,
		"generatedBy": "a later release",
		"foreignKeys": [
			{
				"name": "fk1",
				"onDelete": "CASCADE",
				"table": {"schema": "s", "name": "a", "catalog": "db"},
				"columns": ["x"],
				"refTable": {"schema": "s", "name": "b", "kind": "table"},
				"refColumns": ["id"]
			}
		]
	}`)
	got, err := LoadModelForeignKeys(root, "m")
	require.NoError(t, err)
	assert.Equal(t, []datatug.TableForeignKeys{{Table: refsTable("s", "a"), ForeignKeys: datatug.ForeignKeys{
		refsKey("fk1", []string{"x"}, refsTable("s", "b"), "id"),
	}}}, got)
}

func TestModelForeignKeys_LoadReturnsTheCanonicalOrderWhateverTheFileHas(t *testing.T) {
	root := t.TempDir()
	writeRaw(t, nestedRefsPath(root, "m"), `{"version":1,"foreignKeys":[
		{"name":"b","table":{"name":"t"},"columns":["x"],"refTable":{"name":"u"},"refColumns":["id"]},
		{"name":"a","table":{"name":"t"},"columns":["y","x"],"refTable":{"name":"u"},"refColumns":["j","i"]}
	]}`)
	got, err := LoadModelForeignKeys(root, "m")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].ForeignKeys, 2)
	assert.Equal(t, "a", got[0].ForeignKeys[0].Name)
	assert.Equal(t, []string{"y", "x"}, got[0].ForeignKeys[0].Columns, "the order of the columns is the key's, never sorted")
	assert.Equal(t, []string{"j", "i"}, got[0].ForeignKeys[0].RefColumns)
}

// Every refusal of a file names it inside the project, never where the
// project is on the machine.
func TestModelForeignKeys_LoadRefusals(t *testing.T) {
	const file = "dbmodels/m/m.refs.json"
	const one = `{"name":"fk","table":{"name":"a"},"columns":["x","y"],"refTable":{"name":"b"},"refColumns":["i","j"]}`
	entry := func() string { return `{"version":1,"foreignKeys":[` + one + `]}` }
	cases := map[string]struct{ content, want string }{
		"not_json":                 {"this is not json", "is not a JSON object of the expected form"},
		"empty_file":               {"", "is not a JSON object of the expected form"},
		"json_array":               {"[1]", "is not a JSON object of the expected form"},
		"trailing_data":            {`{"version":1,"foreignKeys":[]} {}`, "is not a JSON object of the expected form"},
		"keys_not_a_list":          {`{"version":1,"foreignKeys":{}}`, "is not a JSON object of the expected form"},
		"version_not_a_number":     {`{"version":"1","foreignKeys":[]}`, "is not a JSON object of the expected form"},
		"no_version":               {`{"foreignKeys":[]}`, `has no "version"`},
		"unknown_version":          {`{"version":2,"foreignKeys":[]}`, "is version 2, which this release does not know (it reads version 1)"},
		"version_zero":             {`{"version":0,"foreignKeys":[]}`, "is version 0, which this release does not know"},
		"unknown_version_new_body": {`{"version":2,"foreignKeys":"in another shape"}`, "is version 2, which this release does not know"},
		"no_name":                  {strings.Replace(entry(), `"name":"fk",`, "", 1), "foreign key at index 0 has no name"},
		"no_table_name":            {strings.Replace(entry(), `"table":{"name":"a"}`, `"table":{}`, 1), "has no table name"},
		"no_columns":               {strings.Replace(entry(), `"columns":["x","y"]`, `"columns":[]`, 1), "has no columns"},
		"empty_column_name":        {strings.Replace(entry(), `"x","y"`, `"x",""`, 1), "has an empty column name"},
		"no_ref_table_name":        {strings.Replace(entry(), `"refTable":{"name":"b"}`, `"refTable":{}`, 1), "has no referenced table name"},
		"no_ref_columns":           {strings.Replace(entry(), `,"refColumns":["i","j"]`, "", 1), "has no referenced columns"},
		"ref_column_count_differs": {strings.Replace(entry(), `"i","j"`, `"i"`, 1), "2 columns but 1 referenced columns"},
		"empty_ref_column_name":    {strings.Replace(entry(), `"i","j"`, `"i",""`, 1), "has an empty referenced column name"},
		"duplicate":                {`{"version":1,"foreignKeys":[` + one + `,` + one + `]}`, "twice"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeRaw(t, filepath.Join(root, filepath.FromSlash(file)), c.content)
			_, err := LoadModelForeignKeys(root, "m")
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
			assert.Contains(t, err.Error(), file)
			assert.NotContains(t, err.Error(), root)
		})
	}
}

func TestModelForeignKeys_LoadRefusesAFileThatIsNotAPlainFile(t *testing.T) {
	t.Run("folder", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(nestedRefsPath(root, "m"), 0o755))
		_, err := LoadModelForeignKeys(root, "m")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dbmodels/m/m.refs.json")
		assert.Contains(t, err.Error(), "not a regular file")
		assert.NotContains(t, err.Error(), root)
	})

	t.Run("link", func(t *testing.T) {
		skipSymlinksOnWindows(t)
		root := t.TempDir()
		outside := filepath.Join(t.TempDir(), "elsewhere.json")
		writeRaw(t, outside, `{"version":1,"foreignKeys":[]}`)
		require.NoError(t, os.MkdirAll(filepath.Dir(nestedRefsPath(root, "m")), 0o755))
		require.NoError(t, os.Symlink(outside, nestedRefsPath(root, "m")))
		_, err := LoadModelForeignKeys(root, "m")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dbmodels/m/m.refs.json")
		assert.Contains(t, err.Error(), "symlink")
		assert.NotContains(t, err.Error(), root)
		assert.NotContains(t, err.Error(), outside, "where a link leads")
	})

	t.Run("over_the_size_limit", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Dir(nestedRefsPath(root, "m")), 0o755))
		f, err := os.Create(nestedRefsPath(root, "m"))
		require.NoError(t, err)
		require.NoError(t, f.Truncate(maxForeignKeysFileSize+1))
		require.NoError(t, f.Close())
		_, err = LoadModelForeignKeys(root, "m")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dbmodels/m/m.refs.json")
		assert.NotContains(t, err.Error(), root)
	})

	t.Run("the_cause_is_kept_for_errors_Is", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(nestedRefsPath(root, "m"), 0o755))
		_, err := LoadModelForeignKeys(root, "m")
		var entryErr *nonRegularEntryError
		assert.True(t, errors.As(err, &entryErr))
	})
}

func TestModelForeignKeys_ModelIdThatNamesNoFile(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"", ".", "..", "a/b", `a\b`, "../outside"} {
		t.Run(id, func(t *testing.T) {
			_, err := LoadModelForeignKeys(root, id)
			assert.ErrorContains(t, err, "cannot name a foreign keys file")
			err = SaveModelForeignKeys(root, id, refsFixture())
			assert.ErrorContains(t, err, "cannot name a foreign keys file")
		})
	}
	assert.Equal(t, 0, len(mustReadDir(t, root)))
}

func mustReadDir(t *testing.T, dir string) []fs.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	return entries
}

// A writer refuses before it touches the file.
func TestModelForeignKeys_SaveRefusals(t *testing.T) {
	other := refsTable("s", "b")
	key := func(mutate func(*datatug.ForeignKey)) []datatug.TableForeignKeys {
		fk := refsKey("fk", []string{"x", "y"}, other, "i", "j")
		mutate(fk)
		return []datatug.TableForeignKeys{{Table: refsTable("s", "a"), ForeignKeys: datatug.ForeignKeys{fk}}}
	}

	cases := map[string]struct {
		keys []datatug.TableForeignKeys
		want string
	}{
		"no_name":           {key(func(fk *datatug.ForeignKey) { fk.Name = "" }), "has no name"},
		"no_columns":        {key(func(fk *datatug.ForeignKey) { fk.Columns = nil; fk.RefColumns = nil }), "has no columns"},
		"empty_column":      {key(func(fk *datatug.ForeignKey) { fk.Columns[1] = "" }), "has an empty column name"},
		"no_ref_columns":    {key(func(fk *datatug.ForeignKey) { fk.RefColumns = nil }), "does not report the referenced columns"},
		"ref_count_differs": {key(func(fk *datatug.ForeignKey) { fk.RefColumns = fk.RefColumns[:1] }), "2 columns but 1 referenced columns"},
		"empty_ref_column":  {key(func(fk *datatug.ForeignKey) { fk.RefColumns[0] = "" }), "has an empty referenced column name"},
		"no_ref_table":      {key(func(fk *datatug.ForeignKey) { fk.RefTable = datatug.DBCollectionKey{} }), "has no referenced table name"},
		"table_has_no_name": {[]datatug.TableForeignKeys{{Table: datatug.DBCollectionKey{}, ForeignKeys: datatug.ForeignKeys{
			refsKey("fk", []string{"x"}, other, "i")}}}, "has no table name"},
		"duplicate": {[]datatug.TableForeignKeys{{Table: refsTable("s", "a"), ForeignKeys: datatug.ForeignKeys{
			refsKey("fk", []string{"x"}, other, "i"), refsKey("fk", []string{"y"}, other, "j")}}}, "twice"},
		"duplicate_in_two_entries": {[]datatug.TableForeignKeys{
			{Table: refsTable("s", "a"), ForeignKeys: datatug.ForeignKeys{refsKey("fk", []string{"x"}, other, "i")}},
			{Table: refsTable("s", "a"), ForeignKeys: datatug.ForeignKeys{refsKey("fk", []string{"x"}, other, "i")}},
		}, "twice"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			err := SaveModelForeignKeys(root, "m", c.keys)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
			assert.Contains(t, err.Error(), "dbmodels/m/m.refs.json")
			assert.Equal(t, 0, len(mustReadDir(t, root)), "nothing was written")
		})
	}

	t.Run("a_key_into_another_catalog", func(t *testing.T) {
		root := t.TempDir()
		inCatalog := func(catalog, name string) datatug.DBCollectionKey {
			return datatug.NewTableKey(name, "s", catalog, nil)
		}
		keys := []datatug.TableForeignKeys{{Table: inCatalog("db1", "a"), ForeignKeys: datatug.ForeignKeys{
			refsKey("fk", []string{"x"}, inCatalog("db2", "b"), "id")}}}
		err := SaveModelForeignKeys(root, "m", keys)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "another catalog")
		assert.Equal(t, 0, len(mustReadDir(t, root)))
	})

	t.Run("a_nil_key_is_skipped", func(t *testing.T) {
		root := t.TempDir()
		keys := []datatug.TableForeignKeys{{Table: refsTable("s", "a"), ForeignKeys: datatug.ForeignKeys{nil,
			refsKey("fk", []string{"x"}, other, "i")}}}
		require.NoError(t, SaveModelForeignKeys(root, "m", keys))
		got, err := LoadModelForeignKeys(root, "m")
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Len(t, got[0].ForeignKeys, 1)
	})

	t.Run("a_refusal_keeps_the_file_of_an_earlier_save", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, SaveModelForeignKeys(root, "m", refsFixture()))
		before := readBytes(t, nestedRefsPath(root, "m"))
		require.Error(t, SaveModelForeignKeys(root, "m", key(func(fk *datatug.ForeignKey) { fk.Name = "" })))
		assert.Equal(t, string(before), string(readBytes(t, nestedRefsPath(root, "m"))))
	})
}

// The write goes through plain files and plain folders of the project only.
func TestModelForeignKeys_SaveRefusesALinkAndAFolderInTheFilesPlace(t *testing.T) {
	const file = "dbmodels/m/m.refs.json"

	t.Run("live_link", func(t *testing.T) {
		f := newLinkFixture(t)
		f.mkdirsAbove(t, file)
		f.link(t, filepath.Join(f.outside, "keep.txt"), file)
		before := treeHash(t, f.outside)
		err := SaveModelForeignKeys(f.root, "m", refsFixture())
		require.ErrorIs(t, err, plainfs.ErrNotPlain)
		f.requireRefusedAndOutsideIntact(t, err, before, false)
		assert.Contains(t, err.Error(), file)
	})

	t.Run("dangling_link", func(t *testing.T) {
		f := newLinkFixture(t)
		f.mkdirsAbove(t, file)
		f.link(t, filepath.Join(f.outside, "not-there.json"), file)
		before := treeHash(t, f.outside)
		err := SaveModelForeignKeys(f.root, "m", refsFixture())
		require.ErrorIs(t, err, plainfs.ErrNotPlain)
		f.requireRefusedAndOutsideIntact(t, err, before, false)
	})

	t.Run("link_in_the_folder_above", func(t *testing.T) {
		f := newLinkFixture(t)
		require.NoError(t, os.MkdirAll(filepath.Join(f.root, "dbmodels"), 0o755))
		outsideDir := filepath.Join(f.outside, "dir")
		f.mirror(t, outsideDir, "m.refs.json")
		f.link(t, outsideDir, "dbmodels/m")
		before := treeHash(t, f.outside)
		err := SaveModelForeignKeys(f.root, "m", refsFixture())
		require.ErrorIs(t, err, plainfs.ErrNotPlain)
		f.requireRefusedAndOutsideIntact(t, err, before, false)
	})

	t.Run("folder", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(nestedRefsPath(root, "m"), 0o755))
		err := SaveModelForeignKeys(root, "m", refsFixture())
		require.ErrorIs(t, err, plainfs.ErrNotPlain)
		assert.Contains(t, err.Error(), file)
	})
}
