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
	"github.com/stretchr/testify/require"
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
			// The saver strips Text out of the JSON sidecar into its own
			// "<id>.query.<type>" file; the loader must read it back.
			assert.Equal(t, "SELECT * FROM users", q.Text)
		})

		t.Run("LoadQueries", func(t *testing.T) {
			folder, err := store.LoadQueries(ctx, folder1) // Pass folder1 here
			assert.NoError(t, err)
			assert.NotNil(t, folder)
			require.Len(t, folder.Items, 1)
			assert.Equal(t, "SELECT * FROM users", folder.Items[0].Text)
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

	t.Run("CreateQuery_DTQL", func(t *testing.T) {
		const dtqlQueryID = "customer-invoices"
		query := datatug.QueryDefWithFolderPath{
			FolderPath: folder1,
			QueryDef: datatug.QueryDef{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{
					ID: dtqlQueryID, Title: "Customer invoices"}},
				Type: datatug.QueryTypeDTQL,
				Text: "from:\n  name: Invoice\n",
			},
		}
		_, err := store.CreateQuery(ctx, query)
		assert.NoError(t, err)

		jsonFileName := fmt.Sprintf("%s.%s.json", dtqlQueryID, storage.QueryFileSuffix)
		assert.FileExists(t, filepath.Join(queriesDir, folder1, jsonFileName))

		// DTQL body follows the same "<id>.query.<lowercase type>" convention as
		// SQL/HTTP query text sidecars - see pkg/datatug/doc.go.
		dtqlFileName := fmt.Sprintf("%s.%s.dtql", dtqlQueryID, storage.QueryFileSuffix)
		assert.FileExists(t, filepath.Join(queriesDir, folder1, dtqlFileName))

		q, err := store.LoadQuery(ctx, path.Join(folder1, dtqlQueryID))
		assert.NoError(t, err)
		assert.Equal(t, datatug.QueryTypeDTQL, q.Type)
		assert.Equal(t, "from:\n  name: Invoice\n", q.Text)
	})

	t.Run("LoadQuery_without_text_sidecar", func(t *testing.T) {
		query := datatug.QueryDefWithFolderPath{
			FolderPath: folder1,
			QueryDef: datatug.QueryDef{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{
					ID: "no-text-query", Title: "No text"}},
				Type: datatug.QueryTypeSQL,
			},
		}
		_, err := store.CreateQuery(ctx, query)
		require.NoError(t, err)

		q, err := store.LoadQuery(ctx, path.Join(folder1, "no-text-query"))
		assert.NoError(t, err)
		assert.Equal(t, "", q.Text)
	})

	// A query.json written without a "type" field (not reachable through
	// CreateQuery/SaveQuery - QueryDef.Validate requires Type) must not make
	// readQueryTextSidecar try to read a "<id>.query." file with an empty
	// extension.
	t.Run("LoadQuery_without_type", func(t *testing.T) {
		require.NoError(t, os.MkdirAll(filepath.Join(queriesDir, folder1), 0777))
		require.NoError(t, os.WriteFile(
			filepath.Join(queriesDir, folder1, "untyped-query.query.json"),
			[]byte(`{"id":"untyped-query","title":"Untyped"}`), 0644))

		q, err := store.LoadQuery(ctx, path.Join(folder1, "untyped-query"))
		assert.NoError(t, err)
		assert.Equal(t, "", q.Text)
	})
}
