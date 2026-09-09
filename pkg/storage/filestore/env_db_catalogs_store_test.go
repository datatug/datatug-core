package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFsEnvCatalogStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_envcatalog")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	projectPath := tmpDir
	envID := "dev"
	serverID := "sqlserver:localhost:1433"
	catalogID := "db1"

	store := newFsEnvCatalogsStore(projectPath)
	ctx := context.Background()

	catalog1 := &datatug.DbCatalog{
		DbCatalogBase: datatug.DbCatalogBase{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					ID:    catalogID,
					Title: "Database 1",
				},
			},
			Driver: "sqlserver",
		},
	}

	t.Run("SaveEnvDbCatalog", func(t *testing.T) {
		err := store.SaveEnvDbCatalog(ctx, envID, serverID, catalogID, catalog1)
		assert.NoError(t, err)

		// SaveEnvDbCatalog/LoadEnvDbCatalogs share one location:
		// environments/<envID>/catalogs/ (serverID plays no part in the path -
		// datatug-demo-projects' catalogs live directly under
		// environments/<envID>/catalogs/, with no servers/<serverID> segment).
		// A brand-new catalog defaults to the nested layout,
		// "<catalogsDir>/<id>/<id>.db.json".
		catalogPath := path.Join(tmpDir, "environments", envID, "catalogs", catalogID, catalogID+"."+storage.DbCatalogFileSuffix+".json")
		_, err = os.Stat(catalogPath)
		assert.NoError(t, err)
	})

	t.Run("LoadEnvDbCatalog", func(t *testing.T) {
		loadedCatalog, err := store.LoadEnvDbCatalog(ctx, envID, serverID, catalogID)
		assert.NoError(t, err)
		assert.Equal(t, catalog1.ID, loadedCatalog.ID)
		assert.Equal(t, catalog1.Title, loadedCatalog.Title)
	})

	t.Run("LoadEnvDbCatalogs", func(t *testing.T) {
		items, err := store.LoadEnvDbCatalogs(ctx, envID)
		assert.NoError(t, err)
		if assert.Len(t, items, 1) {
			assert.Equal(t, catalogID, items[0].ID)
		}
	})

	t.Run("SaveEnvDbCatalogs", func(t *testing.T) {
		catalog2 := &datatug.DbCatalog{
			DbCatalogBase: datatug.DbCatalogBase{
				ProjectItem: datatug.ProjectItem{
					ProjItemBrief: datatug.ProjItemBrief{
						ID:    "db2",
						Title: "Database 2",
					},
				},
				Driver: "sqlserver",
			},
		}
		err := store.SaveEnvDbCatalogs(ctx, envID, serverID, "", datatug.DbCatalogs{catalog1, catalog2})
		assert.NoError(t, err)
	})

	t.Run("DeleteEnvDbCatalog", func(t *testing.T) {
		err := store.DeleteEnvDbCatalog(ctx, envID, serverID, catalogID)
		assert.NoError(t, err)
	})
}

// TestFsEnvCatalogStore_DualLayout mirrors TestFsEntitiesStore_DualLayout for
// env DB catalogs: flat "<catalogsDir>/<id>.db.json" and nested
// "<catalogsDir>/<id>/<id>.db.json" (what datatug-demo-projects uses, e.g.
// environments/local/catalogs/chinook-local/chinook-local.db.json).
func TestFsEnvCatalogStore_DualLayout(t *testing.T) {
	newStore := func(t *testing.T) (fsEnvDbCatalogStore, string) {
		t.Helper()
		tmpDir, err := os.MkdirTemp("", "datatug_envcatalog_layout_test")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
		catalogsDir := path.Join(tmpDir, "environments", "local", "catalogs")
		require.NoError(t, os.MkdirAll(catalogsDir, 0777))
		return newFsEnvCatalogsStore(tmpDir), catalogsDir
	}

	writeFlat := func(t *testing.T, catalogsDir string, c *datatug.DbCatalog) {
		t.Helper()
		data, err := json.Marshal(c)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path.Join(catalogsDir, c.ID+".db.json"), data, 0644))
	}
	writeNested := func(t *testing.T, catalogsDir string, c *datatug.DbCatalog) {
		t.Helper()
		dir := path.Join(catalogsDir, c.ID)
		require.NoError(t, os.MkdirAll(dir, 0777))
		data, err := json.Marshal(c)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path.Join(dir, c.ID+".db.json"), data, 0644))
	}
	newCatalog := func(id string) *datatug.DbCatalog {
		return &datatug.DbCatalog{DbCatalogBase: datatug.DbCatalogBase{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: id}},
			Driver:      "sqlserver",
		}}
	}

	t.Run("loads_flat_only", func(t *testing.T) {
		store, catalogsDir := newStore(t)
		writeFlat(t, catalogsDir, newCatalog("c1"))

		c, err := store.LoadEnvDbCatalog(context.Background(), "local", "", "c1")
		assert.NoError(t, err)
		assert.Equal(t, "c1", c.ID)
	})

	t.Run("loads_nested_only", func(t *testing.T) {
		store, catalogsDir := newStore(t)
		writeNested(t, catalogsDir, newCatalog("chinook-local"))

		c, err := store.LoadEnvDbCatalog(context.Background(), "local", "", "chinook-local")
		assert.NoError(t, err)
		assert.Equal(t, "chinook-local", c.ID)
	})

	t.Run("nested_wins_when_both_agree", func(t *testing.T) {
		store, catalogsDir := newStore(t)
		c := newCatalog("c1")
		writeFlat(t, catalogsDir, c)
		writeNested(t, catalogsDir, c)

		got, err := store.LoadEnvDbCatalog(context.Background(), "local", "", "c1")
		assert.NoError(t, err)
		assert.Equal(t, "c1", got.ID)
	})

	t.Run("conflicting_duplicate_is_a_clear_error", func(t *testing.T) {
		store, catalogsDir := newStore(t)
		flat := newCatalog("c1")
		flat.Title = "Flat"
		nested := newCatalog("c1")
		nested.Title = "Nested"
		writeFlat(t, catalogsDir, flat)
		writeNested(t, catalogsDir, nested)

		_, err := store.LoadEnvDbCatalog(context.Background(), "local", "", "c1")
		assert.Error(t, err)
	})

	t.Run("not_found_in_either_layout", func(t *testing.T) {
		store, _ := newStore(t)
		_, err := store.LoadEnvDbCatalog(context.Background(), "local", "", "missing")
		assert.Error(t, err)
	})

	t.Run("loadEnvDbCatalogs_merges_both_layouts_deduped_and_sorted", func(t *testing.T) {
		store, catalogsDir := newStore(t)
		writeFlat(t, catalogsDir, newCatalog("zzz"))
		writeNested(t, catalogsDir, newCatalog("aaa"))

		catalogs, err := store.LoadEnvDbCatalogs(context.Background(), "local")
		require.NoError(t, err)
		require.Len(t, catalogs, 2)
		assert.Equal(t, "aaa", catalogs[0].ID)
		assert.Equal(t, "zzz", catalogs[1].ID)
	})

	t.Run("save_new_defaults_to_nested", func(t *testing.T) {
		store, catalogsDir := newStore(t)
		require.NoError(t, store.SaveEnvDbCatalog(context.Background(), "local", "", "c1", newCatalog("c1")))

		assert.FileExists(t, path.Join(catalogsDir, "c1", "c1.db.json"))
	})

	t.Run("loadEnvDbCatalogs_propagates_a_conflicting_duplicate_error", func(t *testing.T) {
		store, catalogsDir := newStore(t)
		flat := newCatalog("c1")
		flat.Title = "Flat"
		nested := newCatalog("c1")
		nested.Title = "Nested"
		writeFlat(t, catalogsDir, flat)
		writeNested(t, catalogsDir, nested)

		_, err := store.LoadEnvDbCatalogs(context.Background(), "local")
		assert.Error(t, err)
	})

	t.Run("loadEnvDbCatalogs_skips_a_directory_with_neither_file", func(t *testing.T) {
		store, catalogsDir := newStore(t)
		writeFlat(t, catalogsDir, newCatalog("c1"))
		require.NoError(t, os.MkdirAll(path.Join(catalogsDir, "empty"), 0777))

		catalogs, err := store.LoadEnvDbCatalogs(context.Background(), "local")
		require.NoError(t, err)
		require.Len(t, catalogs, 1)
		assert.Equal(t, "c1", catalogs[0].ID)
	})

	t.Run("saveEnvDbCatalog_propagates_invalid_data_error", func(t *testing.T) {
		store, _ := newStore(t)
		err := store.SaveEnvDbCatalog(context.Background(), "local", "", "c1", &datatug.DbCatalog{})
		assert.Error(t, err)
	})

	t.Run("delete_removes_from_whichever_layout_exists", func(t *testing.T) {
		store, catalogsDir := newStore(t)
		writeFlat(t, catalogsDir, newCatalog("flat1"))
		writeNested(t, catalogsDir, newCatalog("nested1"))

		require.NoError(t, store.DeleteEnvDbCatalog(context.Background(), "local", "", "flat1"))
		require.NoError(t, store.DeleteEnvDbCatalog(context.Background(), "local", "", "nested1"))

		assert.NoFileExists(t, path.Join(catalogsDir, "flat1.db.json"))
		assert.NoFileExists(t, path.Join(catalogsDir, "nested1", "nested1.db.json"))
	})
}
