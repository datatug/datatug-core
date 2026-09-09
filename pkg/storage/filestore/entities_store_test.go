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

func TestFsEntitiesStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_entities_test")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	entitiesDir := filepath.Join(tmpDir, storage.EntitiesFolder)
	err = os.MkdirAll(entitiesDir, 0777)
	assert.NoError(t, err)

	store := fsEntitiesStore{
		fsProjectItemsStore: fsProjectItemsStore[datatug.Entities, *datatug.Entity, datatug.Entity]{
			dirPath:        entitiesDir,
			itemFileSuffix: storage.EntityFileSuffix,
		},
	}
	ctx := context.Background()

	t.Run("saveEntity", func(t *testing.T) {
		entity := &datatug.Entity{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "entity1"}},
		}
		err := store.SaveEntity(ctx, entity)
		assert.NoError(t, err)
		// A brand-new entity defaults to the nested per-entity-directory
		// layout, matching datatug-cli and datatug-demo-projects.
		assert.FileExists(t, filepath.Join(entitiesDir, "entity1", "entity1.entity.json"))
	})

	t.Run("loadEntity", func(t *testing.T) {
		e, err := store.LoadEntity(ctx, "entity1")
		assert.NoError(t, err)
		assert.Equal(t, "entity1", e.ID)
	})

	t.Run("loadEntities", func(t *testing.T) {
		entities, err := store.LoadEntities(ctx)
		assert.NoError(t, err)
		assert.Len(t, entities, 1)
	})

	t.Run("deleteEntity", func(t *testing.T) {
		err := store.DeleteEntity(ctx, "entity1")
		assert.NoError(t, err)
	})

	t.Run("saveEntities", func(t *testing.T) {
		entities := datatug.Entities{
			{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "entity2"}}},
		}
		err := store.SaveEntities(ctx, entities)
		assert.NoError(t, err)
	})

	t.Run("saveAndLoadEntity_withMappings", func(t *testing.T) {
		entity := &datatug.Entity{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customer"}},
			Fields: datatug.EntityFields{
				{
					ID:   "id",
					Type: "string",
					Mappings: datatug.PhysicalRefs{
						{Source: "chinook", Collection: "Customer", Column: "CustomerId"},
						{Source: "support-notes", Collection: "Customer", Column: "CustomerId"},
					},
				},
			},
		}
		err := store.SaveEntity(ctx, entity)
		assert.NoError(t, err)

		loaded, err := store.LoadEntity(ctx, "customer")
		assert.NoError(t, err)
		assert.Equal(t, entity.Fields, loaded.Fields)
	})
}

// TestFsEntitiesStore_DualLayout covers the layout-compatibility rules a
// filestore consumer (datatug-cli, datatug-demo-projects) depends on: load
// from either the legacy flat "<id>.entity.json" layout or the nested
// "<id>/<id>.entity.json" layout datatug-cli's vendored copy and
// datatug-demo-projects already use, with nested winning when both exist and
// agree, and a clear error when they disagree.
func TestFsEntitiesStore_DualLayout(t *testing.T) {
	newStore := func(t *testing.T) (fsEntitiesStore, string) {
		t.Helper()
		tmpDir, err := os.MkdirTemp("", "datatug_entities_layout_test")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
		entitiesDir := filepath.Join(tmpDir, storage.EntitiesFolder)
		require.NoError(t, os.MkdirAll(entitiesDir, 0777))
		return newFsEntitiesStore(tmpDir), entitiesDir
	}

	writeFlat := func(t *testing.T, entitiesDir string, entity *datatug.Entity) {
		t.Helper()
		data, err := json.Marshal(entity)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(entitiesDir, entity.ID+".entity.json"), data, 0644))
	}
	writeNested := func(t *testing.T, entitiesDir string, entity *datatug.Entity) {
		t.Helper()
		dir := filepath.Join(entitiesDir, entity.ID)
		require.NoError(t, os.MkdirAll(dir, 0777))
		data, err := json.Marshal(entity)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, entity.ID+".entity.json"), data, 0644))
	}

	t.Run("loads_flat_only", func(t *testing.T) {
		store, entitiesDir := newStore(t)
		writeFlat(t, entitiesDir, &datatug.Entity{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1"}}})

		e, err := store.LoadEntity(context.Background(), "e1")
		assert.NoError(t, err)
		assert.Equal(t, "e1", e.ID)
	})

	t.Run("loads_nested_only", func(t *testing.T) {
		store, entitiesDir := newStore(t)
		writeNested(t, entitiesDir, &datatug.Entity{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1"}}})

		e, err := store.LoadEntity(context.Background(), "e1")
		assert.NoError(t, err)
		assert.Equal(t, "e1", e.ID)
	})

	t.Run("nested_wins_when_both_agree", func(t *testing.T) {
		store, entitiesDir := newStore(t)
		entity := &datatug.Entity{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1"}}}
		writeFlat(t, entitiesDir, entity)
		writeNested(t, entitiesDir, entity)

		e, err := store.LoadEntity(context.Background(), "e1")
		assert.NoError(t, err)
		assert.Equal(t, "e1", e.ID)
	})

	t.Run("conflicting_duplicate_is_a_clear_error", func(t *testing.T) {
		store, entitiesDir := newStore(t)
		writeFlat(t, entitiesDir, &datatug.Entity{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1", Title: "Flat version"}}})
		writeNested(t, entitiesDir, &datatug.Entity{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1", Title: "Nested version"}}})

		_, err := store.LoadEntity(context.Background(), "e1")
		assert.Error(t, err)
	})

	t.Run("not_found_in_either_layout", func(t *testing.T) {
		store, _ := newStore(t)
		_, err := store.LoadEntity(context.Background(), "missing")
		assert.Error(t, err)
	})

	t.Run("loadEntities_merges_both_layouts_deduped_and_sorted", func(t *testing.T) {
		store, entitiesDir := newStore(t)
		writeFlat(t, entitiesDir, &datatug.Entity{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "zzz"}}})
		writeNested(t, entitiesDir, &datatug.Entity{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "aaa"}}})
		bothLayouts := &datatug.Entity{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "mmm"}}}
		writeFlat(t, entitiesDir, bothLayouts)
		writeNested(t, entitiesDir, bothLayouts)

		entities, err := store.LoadEntities(context.Background())
		require.NoError(t, err)
		require.Len(t, entities, 3)
		assert.Equal(t, []string{"aaa", "mmm", "zzz"}, entities.IDs())
	})

	t.Run("save_preserves_flat_layout_it_was_loaded_from", func(t *testing.T) {
		store, entitiesDir := newStore(t)
		writeFlat(t, entitiesDir, &datatug.Entity{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1"}}})

		e, err := store.LoadEntity(context.Background(), "e1")
		require.NoError(t, err)
		e.Title = "Updated"
		require.NoError(t, store.SaveEntity(context.Background(), e))

		assert.FileExists(t, filepath.Join(entitiesDir, "e1.entity.json"))
		assert.NoFileExists(t, filepath.Join(entitiesDir, "e1", "e1.entity.json"))
	})

	t.Run("save_preserves_nested_layout_it_was_loaded_from", func(t *testing.T) {
		store, entitiesDir := newStore(t)
		writeNested(t, entitiesDir, &datatug.Entity{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1"}}})

		e, err := store.LoadEntity(context.Background(), "e1")
		require.NoError(t, err)
		e.Title = "Updated"
		require.NoError(t, store.SaveEntity(context.Background(), e))

		assert.FileExists(t, filepath.Join(entitiesDir, "e1", "e1.entity.json"))
		assert.NoFileExists(t, filepath.Join(entitiesDir, "e1.entity.json"))
	})

	t.Run("delete_removes_from_whichever_layout_exists", func(t *testing.T) {
		store, entitiesDir := newStore(t)
		writeFlat(t, entitiesDir, &datatug.Entity{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "flat1"}}})
		writeNested(t, entitiesDir, &datatug.Entity{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "nested1"}}})

		require.NoError(t, store.DeleteEntity(context.Background(), "flat1"))
		require.NoError(t, store.DeleteEntity(context.Background(), "nested1"))

		assert.NoFileExists(t, filepath.Join(entitiesDir, "flat1.entity.json"))
		assert.NoFileExists(t, filepath.Join(entitiesDir, "nested1", "nested1.entity.json"))
	})
}
