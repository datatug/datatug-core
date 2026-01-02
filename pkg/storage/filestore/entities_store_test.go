package filestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
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
		assert.FileExists(t, filepath.Join(entitiesDir, "entity1.entity.json"))
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
}
