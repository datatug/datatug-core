package filestore

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestFsQueriesStore(t *testing.T) {
	var projectDir string
	{
		var err error
		projectDir, err = os.MkdirTemp("", "datatug_queries_test")
		assert.NoError(t, err)
		defer func() { _ = os.RemoveAll(projectDir) }()
	}

	store := newFsQueriesStore(projectDir)
	ctx := context.Background()

	queriesDir := path.Join(projectDir, storage.QueriesFolder)

	t.Run("CreateQueryFolder", func(t *testing.T) {
		err := store.CreateQueryFolder(ctx, "", "folder1")
		assert.NoError(t, err)
		assert.DirExists(t, filepath.Join(queriesDir, "folder1"))
		assert.FileExists(t, filepath.Join(queriesDir, "folder1", "README.md"))
	})

	const folder1 = "folder1"
	const query1ID = "query1"
	var query1fullID = path.Join(folder1, query1ID)

	t.Run("CreateQuery_SQL", func(t *testing.T) {
		query := datatug.QueryDefWithFolderPath{
			FolderPath: folder1, // Removed trailing slash
			QueryDef: datatug.QueryDef{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{
					ID: query1ID, Title: "Query 1"}},
				Type: datatug.QueryTypeSQL,
				Text: "SELECT * FROM users",
			},
		}
		_, err := store.CreateQuery(ctx, query)
		assert.NoError(t, err)

		jsonFileName := fmt.Sprintf("%s.%s.json", query1ID, storage.QueryFileSuffix)
		assert.FileExists(t, filepath.Join(queriesDir, folder1, jsonFileName))

		sqlFileName := fmt.Sprintf("%s.%s.sql", query1ID, storage.QueryFileSuffix)
		assert.FileExists(t, filepath.Join(queriesDir, folder1, sqlFileName))

		t.Run("LoadQuery", func(t *testing.T) {
			q, err := store.LoadQuery(ctx, query1fullID)
			assert.NoError(t, err)
			assert.NotNil(t, q)
			assert.Equal(t, query1ID, q.ID)
			assert.Equal(t, "", q.Folder)
		})

		t.Run("LoadQueries", func(t *testing.T) {
			folder, err := store.LoadQueries(ctx, folder1) // Pass folder1 here
			assert.NoError(t, err)
			assert.NotNil(t, folder)
			assert.Len(t, folder.Items, 1)
		})

		t.Run("UpdateQuery", func(t *testing.T) {
			query := datatug.QueryDef{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{
					ID: query1fullID, Title: "Query 1 Updated"}},
				Type: datatug.QueryTypeSQL,
				Text: "SELECT 1",
			}
			_, err := store.UpdateQuery(ctx, query)
			assert.NoError(t, err)
		})

		t.Run("DeleteQuery", func(t *testing.T) {
			err := store.DeleteQuery(ctx, query1fullID)
			assert.NoError(t, err)
		})

		t.Run("SaveQuery", func(t *testing.T) {
			query := &datatug.QueryDefWithFolderPath{
				FolderPath: folder1,
				QueryDef: datatug.QueryDef{
					ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{
						ID: "query2", Title: "Query 2"}},
					Type: datatug.QueryTypeSQL,
					Text: "SELECT * FROM products",
				},
			}
			err := store.SaveQuery(ctx, query)
			assert.NoError(t, err)
			assert.FileExists(t, filepath.Join(queriesDir, "query2.query.json"))
		})
	})
}
