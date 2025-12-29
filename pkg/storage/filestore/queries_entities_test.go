package filestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestFsQueriesStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_queries_test")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	queriesDir := filepath.Join(tmpDir, QueriesFolder)
	err = os.MkdirAll(queriesDir, 0777)
	assert.NoError(t, err)

	store := fsQueriesStore{
		fsProjectItemsStore: fsProjectItemsStore[datatug.QueryDefs, *datatug.QueryDef, datatug.QueryDef]{
			dirPath:        queriesDir,
			itemFileSuffix: querySQLFileSuffix,
		},
	}
	ctx := context.Background()

	t.Run("CreateQueryFolder", func(t *testing.T) {
		err := store.CreateQueryFolder(ctx, "", "folder1")
		assert.NoError(t, err)
		assert.DirExists(t, filepath.Join(queriesDir, "folder1"))
		assert.FileExists(t, filepath.Join(queriesDir, "folder1", "README.md"))
	})

	t.Run("CreateQuery_SQL", func(t *testing.T) {
		query := datatug.QueryDefWithFolderPath{
			FolderPath: "folder1", // Removed trailing slash
			QueryDef: datatug.QueryDef{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "query1.sql", Title: "Query 1"}},
				Type:        "SQL",
				Text:        "SELECT * FROM users",
			},
		}
		_, err := store.CreateQuery(ctx, query)
		if err != nil {
			t.Fatalf("failed to create query: %v", err)
		}
		assert.FileExists(t, filepath.Join(queriesDir, "folder1", "query1.sql.json"))
		assert.FileExists(t, filepath.Join(queriesDir, "folder1", "query1.sql"))
	})

	t.Run("GetQuery", func(t *testing.T) {
		q, err := store.GetQuery(ctx, "folder1/query1.sql")
		if err != nil {
			t.Fatalf("failed to get query: %v", err)
		}
		if q == nil {
			t.Fatal("got nil query")
		}
		assert.Equal(t, "query1", q.ID) // ID should be without .sql
	})

	t.Run("LoadQueries", func(t *testing.T) {
		folder, err := store.LoadQueries(ctx, "folder1") // Pass folder1 here
		assert.NoError(t, err)
		assert.NotNil(t, folder)
		assert.Len(t, folder.Items, 1)
	})

	t.Run("UpdateQuery", func(t *testing.T) {
		query := datatug.QueryDef{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "query1.sql", Title: "Query 1 Updated"}},
			Type:        "SQL",
			Text:        "SELECT 1",
		}
		_, err := store.UpdateQuery(ctx, query)
		assert.NoError(t, err)
	})

	t.Run("DeleteQuery", func(t *testing.T) {
		err := store.DeleteQuery(ctx, "folder1/query1.sql")
		assert.NoError(t, err)
	})

	t.Run("DeleteQueryFolder", func(t *testing.T) {
		err := store.DeleteQueryFolder(ctx, "folder1")
		assert.NoError(t, err)
	})
}

func TestFsEntitiesStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_entities_test")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	entitiesDir := filepath.Join(tmpDir, EntitiesFolder)
	err = os.MkdirAll(entitiesDir, 0777)
	assert.NoError(t, err)

	store := fsEntitiesStore{
		fsProjectItemsStore: fsProjectItemsStore[datatug.Entities, *datatug.Entity, datatug.Entity]{
			dirPath:        entitiesDir,
			itemFileSuffix: entityFileSuffix,
		},
	}
	ctx := context.Background()

	t.Run("saveEntity", func(t *testing.T) {
		entity := &datatug.Entity{
			ProjEntityBrief: datatug.ProjEntityBrief{ProjItemBrief: datatug.ProjItemBrief{ID: "entity1"}},
		}
		err := store.saveEntity(ctx, entity)
		assert.NoError(t, err)
		assert.FileExists(t, filepath.Join(entitiesDir, "entity1.entity.json"))
	})

	t.Run("loadEntity", func(t *testing.T) {
		e, err := store.loadEntity(ctx, "entity1")
		assert.NoError(t, err)
		assert.Equal(t, "entity1", e.ID)
	})

	t.Run("loadEntities", func(t *testing.T) {
		entities, err := store.loadEntities(ctx)
		assert.NoError(t, err)
		assert.Len(t, entities, 1)
	})

	t.Run("deleteEntity", func(t *testing.T) {
		err := store.deleteEntity(ctx, "entity1")
		assert.NoError(t, err)
	})

	t.Run("saveEntities", func(t *testing.T) {
		entities := datatug.Entities{
			{ProjEntityBrief: datatug.ProjEntityBrief{ProjItemBrief: datatug.ProjItemBrief{ID: "entity2"}}},
		}
		err := store.saveEntities(ctx, entities)
		assert.NoError(t, err)
	})
}
