package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFsDbModelsStore_DualLayout mirrors TestFsEntitiesStore_DualLayout: the
// same layout-compatibility rules apply to DB models, whose id-prefixed
// nested filename ("<id>/<id>.dbmodel.json") matches entities exactly - see
// datatug-demo-projects/demo-project-1/dbmodels/chinook/chinook.dbmodel.json.
func TestFsDbModelsStore_DualLayout(t *testing.T) {
	newStore := func(t *testing.T) (fsDbModelsStore, string) {
		t.Helper()
		tmpDir, err := os.MkdirTemp("", "datatug_dbmodels_layout_test")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
		dbModelsDir := filepath.Join(tmpDir, storage.DbModelsFolder)
		require.NoError(t, os.MkdirAll(dbModelsDir, 0777))
		return newFsDbModelsStore(tmpDir), dbModelsDir
	}

	writeFlat := func(t *testing.T, dbModelsDir string, m *datatug.DbModel) {
		t.Helper()
		data, err := json.Marshal(m)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dbModelsDir, m.ID+".dbmodel.json"), data, 0644))
	}
	writeNested := func(t *testing.T, dbModelsDir string, m *datatug.DbModel) {
		t.Helper()
		dir := filepath.Join(dbModelsDir, m.ID)
		require.NoError(t, os.MkdirAll(dir, 0777))
		data, err := json.Marshal(m)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, m.ID+".dbmodel.json"), data, 0644))
	}

	t.Run("loads_flat_only", func(t *testing.T) {
		store, dbModelsDir := newStore(t)
		writeFlat(t, dbModelsDir, &datatug.DbModel{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "m1"}}})

		m, err := store.LoadDbModel(context.Background(), "m1")
		assert.NoError(t, err)
		assert.Equal(t, "m1", m.ID)
	})

	t.Run("loads_nested_only", func(t *testing.T) {
		store, dbModelsDir := newStore(t)
		writeNested(t, dbModelsDir, &datatug.DbModel{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "m1"}}})

		m, err := store.LoadDbModel(context.Background(), "m1")
		assert.NoError(t, err)
		assert.Equal(t, "m1", m.ID)
	})

	t.Run("nested_wins_when_both_agree", func(t *testing.T) {
		store, dbModelsDir := newStore(t)
		m := &datatug.DbModel{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "m1"}}}
		writeFlat(t, dbModelsDir, m)
		writeNested(t, dbModelsDir, m)

		got, err := store.LoadDbModel(context.Background(), "m1")
		assert.NoError(t, err)
		assert.Equal(t, "m1", got.ID)
	})

	t.Run("conflicting_duplicate_is_a_clear_error", func(t *testing.T) {
		store, dbModelsDir := newStore(t)
		writeFlat(t, dbModelsDir, &datatug.DbModel{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "m1", Title: "Flat"}}})
		writeNested(t, dbModelsDir, &datatug.DbModel{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "m1", Title: "Nested"}}})

		_, err := store.LoadDbModel(context.Background(), "m1")
		assert.Error(t, err)
	})

	t.Run("not_found_in_either_layout", func(t *testing.T) {
		store, _ := newStore(t)
		_, err := store.LoadDbModel(context.Background(), "missing")
		assert.Error(t, err)
	})

	t.Run("loadDbModels_merges_both_layouts_deduped_and_sorted", func(t *testing.T) {
		store, dbModelsDir := newStore(t)
		writeFlat(t, dbModelsDir, &datatug.DbModel{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "zzz"}}})
		writeNested(t, dbModelsDir, &datatug.DbModel{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "aaa"}}})

		models, err := store.LoadDbModels(context.Background())
		require.NoError(t, err)
		require.Len(t, models, 2)
		assert.Equal(t, "aaa", models[0].ID)
		assert.Equal(t, "zzz", models[1].ID)
	})

	t.Run("save_preserves_flat_layout_it_was_loaded_from", func(t *testing.T) {
		store, dbModelsDir := newStore(t)
		writeFlat(t, dbModelsDir, &datatug.DbModel{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "m1"}}})

		m, err := store.LoadDbModel(context.Background(), "m1")
		require.NoError(t, err)
		m.Title = "Updated"
		require.NoError(t, store.SaveDbModel(context.Background(), m))

		assert.FileExists(t, filepath.Join(dbModelsDir, "m1.dbmodel.json"))
		assert.NoFileExists(t, filepath.Join(dbModelsDir, "m1", "m1.dbmodel.json"))
	})

	t.Run("save_preserves_nested_layout_it_was_loaded_from", func(t *testing.T) {
		store, dbModelsDir := newStore(t)
		writeNested(t, dbModelsDir, &datatug.DbModel{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "m1"}}})

		m, err := store.LoadDbModel(context.Background(), "m1")
		require.NoError(t, err)
		m.Title = "Updated"
		require.NoError(t, store.SaveDbModel(context.Background(), m))

		assert.FileExists(t, filepath.Join(dbModelsDir, "m1", "m1.dbmodel.json"))
		assert.NoFileExists(t, filepath.Join(dbModelsDir, "m1.dbmodel.json"))
	})

	t.Run("save_new_defaults_to_nested", func(t *testing.T) {
		store, dbModelsDir := newStore(t)
		require.NoError(t, store.SaveDbModel(context.Background(), &datatug.DbModel{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "m1"}}}))

		assert.FileExists(t, filepath.Join(dbModelsDir, "m1", "m1.dbmodel.json"))
	})

	t.Run("delete_removes_from_whichever_layout_exists", func(t *testing.T) {
		store, dbModelsDir := newStore(t)
		writeFlat(t, dbModelsDir, &datatug.DbModel{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "flat1"}}})
		writeNested(t, dbModelsDir, &datatug.DbModel{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "nested1"}}})

		require.NoError(t, store.DeleteDbModel(context.Background(), "flat1"))
		require.NoError(t, store.DeleteDbModel(context.Background(), "nested1"))

		assert.NoFileExists(t, filepath.Join(dbModelsDir, "flat1.dbmodel.json"))
		assert.NoFileExists(t, filepath.Join(dbModelsDir, "nested1", "nested1.dbmodel.json"))
	})
}
