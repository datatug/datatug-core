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

func TestFsQueriesStore(t *testing.T) {
	var queriesDir string
	{
		tmpDir, err := os.MkdirTemp("", "datatug_queries_test")
		assert.NoError(t, err)
		defer func() { _ = os.RemoveAll(tmpDir) }()

		queriesDir = filepath.Join(tmpDir, storage.QueriesFolder)
		err = os.MkdirAll(queriesDir, 0777)
		assert.NoError(t, err)
	}

	store := fsQueriesStore{
		fsProjectItemsStore: fsProjectItemsStore[datatug.QueryDefs, *datatug.QueryDef, datatug.QueryDef]{
			dirPath:        queriesDir,
			itemFileSuffix: "",
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
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{
					ID: "query1", Title: "Query 1"}},
				Type: datatug.QueryTypeSQL,
				Text: "SELECT * FROM users",
			},
		}
		_, err := store.CreateQuery(ctx, query)
		assert.NoError(t, err)
		assert.FileExists(t, filepath.Join(queriesDir, "folder1", "query1.json"))
		assert.FileExists(t, filepath.Join(queriesDir, "folder1", "query1.sql"))
	})

	t.Run("LoadQuery", func(t *testing.T) {
		q, err := store.LoadQuery(ctx, "folder1/query1")
		assert.NoError(t, err)
		assert.NotNil(t, q)
		assert.Equal(t, "query1", q.ID)
		assert.Equal(t, "folder1", q.Folder)
	})

	t.Run("LoadQueries", func(t *testing.T) {
		folder, err := store.LoadQueries(ctx, "folder1") // Pass folder1 here
		assert.NoError(t, err)
		assert.NotNil(t, folder)
		assert.Len(t, folder.Items, 1)
	})

	t.Run("UpdateQuery", func(t *testing.T) {
		query := datatug.QueryDef{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{
				ID: "query1", Title: "Query 1 Updated"}},
			Type: datatug.QueryTypeSQL,
			Text: "SELECT 1",
		}
		_, err := store.UpdateQuery(ctx, query)
		assert.NoError(t, err)
	})

	t.Run("DeleteQuery", func(t *testing.T) {
		err := store.DeleteQuery(ctx, "folder1/query1.sql")
		assert.NoError(t, err)
	})

	t.Run("SaveQuery", func(t *testing.T) {
		query := &datatug.QueryDefWithFolderPath{
			FolderPath: "folder1",
			QueryDef: datatug.QueryDef{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{
					ID: "query2", Title: "Query 2"}},
				Type: datatug.QueryTypeSQL,
				Text: "SELECT * FROM products",
			},
		}
		err := store.SaveQuery(ctx, query)
		assert.NoError(t, err)
		assert.FileExists(t, filepath.Join(queriesDir, "query2.json"))
	})
}
