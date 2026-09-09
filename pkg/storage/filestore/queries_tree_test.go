package filestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFsQueriesStore_LoadQueriesTree covers the recursive folder walk
// LoadProject needs: every sub-folder under queries/ that has at least one
// "*.query.json" item (at any depth) shows up, folders holding only
// unrelated files (datatug-demo-projects/demo-project-1's legacy
// albums/artists/tracks *.sql.json queries, which predate the "query" file
// suffix - see queries/albums/albums_by_title.sql.json) are skipped rather
// than appearing as empty entries, and text sidecars load.
func TestFsQueriesStore_LoadQueriesTree(t *testing.T) {
	newStore := func(t *testing.T) (fsQueriesStore, string) {
		t.Helper()
		tmpDir, err := os.MkdirTemp("", "datatug_queries_tree_test")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
		return newFsQueriesStore(tmpDir), tmpDir
	}
	ctx := context.Background()

	t.Run("empty_project_has_no_queries", func(t *testing.T) {
		store, _ := newStore(t)
		folder, err := store.loadQueriesTree(ctx, "")
		assert.NoError(t, err)
		assert.Nil(t, folder)
	})

	t.Run("flat_query_at_root", func(t *testing.T) {
		store, _ := newStore(t)
		_, err := store.CreateQuery(ctx, datatug.QueryDefWithFolderPath{
			QueryDef: datatug.QueryDef{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q1", Title: "Q1"}},
				Type:        datatug.QueryTypeSQL,
				Text:        "SELECT 1",
			},
		})
		require.NoError(t, err)

		folder, err := store.loadQueriesTree(ctx, "")
		require.NoError(t, err)
		require.NotNil(t, folder)
		require.Len(t, folder.Items, 1)
		assert.Equal(t, "q1", folder.Items[0].ID)
		assert.Equal(t, "SELECT 1", folder.Items[0].Text)
		assert.Empty(t, folder.Folders)
	})

	t.Run("nested_folders_with_queries", func(t *testing.T) {
		store, _ := newStore(t)
		for _, fp := range []string{"customers", "invoices", "reference"} {
			_, err := store.CreateQuery(ctx, datatug.QueryDefWithFolderPath{
				FolderPath: fp,
				QueryDef: datatug.QueryDef{
					ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: fp + "-query", Title: fp}},
					Type:        datatug.QueryTypeSQL,
					Text:        "SELECT 1",
				},
			})
			require.NoError(t, err)
		}

		folder, err := store.loadQueriesTree(ctx, "")
		require.NoError(t, err)
		require.NotNil(t, folder)
		assert.Empty(t, folder.Items, "no queries live directly at the root in this scenario")
		require.Len(t, folder.Folders, 3)
		gotFolderIDs := make([]string, len(folder.Folders))
		for i, f := range folder.Folders {
			gotFolderIDs[i] = f.ID
			require.Len(t, f.Items, 1)
			assert.Equal(t, f.ID+"-query", f.Items[0].ID)
		}
		assert.Equal(t, []string{"customers", "invoices", "reference"}, gotFolderIDs, "folders must be sorted")
	})

	t.Run("folders_with_only_unrelated_files_are_skipped", func(t *testing.T) {
		store, tmpDir := newStore(t)
		_, err := store.CreateQuery(ctx, datatug.QueryDefWithFolderPath{
			FolderPath: "customers",
			QueryDef: datatug.QueryDef{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q1", Title: "Q1"}},
				Type:        datatug.QueryTypeSQL,
				Text:        "SELECT 1",
			},
		})
		require.NoError(t, err)

		// A folder holding only a legacy-shaped, non-"*.query.json" file -
		// matches datatug-demo-projects/demo-project-1's queries/albums/.
		legacyDir := filepath.Join(tmpDir, "queries", "albums")
		require.NoError(t, os.MkdirAll(legacyDir, 0777))
		require.NoError(t, os.WriteFile(filepath.Join(legacyDir, "albums_by_title.sql.json"), []byte(`{"title":"Albums by title"}`), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(legacyDir, "albums_by_title.sql"), []byte("SELECT * FROM albums"), 0644))

		folder, err := store.loadQueriesTree(ctx, "")
		require.NoError(t, err)
		require.NotNil(t, folder)
		require.Len(t, folder.Folders, 1, "the legacy-shaped folder must not appear")
		assert.Equal(t, "customers", folder.Folders[0].ID)
	})

	t.Run("propagates_LoadQueries_error_at_root", func(t *testing.T) {
		store, tmpDir := newStore(t)
		queriesDir := filepath.Join(tmpDir, "queries")
		require.NoError(t, os.MkdirAll(queriesDir, 0777))
		require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "broken.query.json"), []byte("not json"), 0644))

		_, err := store.loadQueriesTree(ctx, "")
		assert.Error(t, err)
	})

	t.Run("propagates_nested_folder_load_error", func(t *testing.T) {
		store, tmpDir := newStore(t)
		nestedDir := filepath.Join(tmpDir, "queries", "customers")
		require.NoError(t, os.MkdirAll(nestedDir, 0777))
		require.NoError(t, os.WriteFile(filepath.Join(nestedDir, "broken.query.json"), []byte("not json"), 0644))

		_, err := store.loadQueriesTree(ctx, "")
		assert.Error(t, err)
	})

	t.Run("deeply_nested_folder", func(t *testing.T) {
		store, _ := newStore(t)
		_, err := store.CreateQuery(ctx, datatug.QueryDefWithFolderPath{
			FolderPath: filepath.Join("a", "b"),
			QueryDef: datatug.QueryDef{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "deep", Title: "Deep"}},
				Type:        datatug.QueryTypeSQL,
				Text:        "SELECT 1",
			},
		})
		require.NoError(t, err)

		folder, err := store.loadQueriesTree(ctx, "")
		require.NoError(t, err)
		require.NotNil(t, folder)
		require.Len(t, folder.Folders, 1)
		a := folder.Folders[0]
		assert.Equal(t, "a", a.ID)
		assert.Empty(t, a.Items)
		require.Len(t, a.Folders, 1)
		b := a.Folders[0]
		assert.Equal(t, "b", b.ID)
		require.Len(t, b.Items, 1)
		assert.Equal(t, "deep", b.Items[0].ID)
	})
}

// TestFsQueriesStore_SaveQueriesTree proves the queries tree round-trips
// through save->load: this is "the query saver must round-trip".
func TestFsQueriesStore_SaveQueriesTree(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_queries_tree_save_test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	store := newFsQueriesStore(tmpDir)
	ctx := context.Background()

	tree := &datatug.QueriesFolder{
		Items: datatug.QueryDefs{
			{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "root-query", Title: "Root query"}},
				Type:        datatug.QueryTypeSQL,
				Text:        "SELECT 1",
			},
		},
		Folders: datatug.QueryFolders{
			{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customers"}},
				Items: datatug.QueryDefs{
					{
						ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customer-invoices", Title: "Customer invoices"}},
						Type:        datatug.QueryTypeDTQL,
						Text:        "select:\n  from: Invoice\n",
						Parameters: datatug.Parameters{
							{ID: "CustomerId", Type: "integer", IsRequired: true, Meta: &datatug.EntityFieldRef{Entity: "Customer", Field: "ID"}},
						},
					},
				},
			},
		},
	}

	require.NoError(t, store.saveQueriesTree(ctx, "", tree))

	loaded, err := store.loadQueriesTree(ctx, "")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Len(t, loaded.Items, 1)
	assert.Equal(t, "root-query", loaded.Items[0].ID)
	assert.Equal(t, "SELECT 1", loaded.Items[0].Text)

	require.Len(t, loaded.Folders, 1)
	customers := loaded.Folders[0]
	assert.Equal(t, "customers", customers.ID)
	require.Len(t, customers.Items, 1)
	invoices := customers.Items[0]
	assert.Equal(t, "customer-invoices", invoices.ID)
	assert.Equal(t, datatug.QueryTypeDTQL, invoices.Type)
	assert.Equal(t, "select:\n  from: Invoice\n", invoices.Text)
	require.Len(t, invoices.Parameters, 1)
	assert.Equal(t, "CustomerId", invoices.Parameters[0].ID)
}

func TestFsQueriesStore_SaveQueriesTree_Nil(t *testing.T) {
	store := newFsQueriesStore(t.TempDir())
	assert.NoError(t, store.saveQueriesTree(context.Background(), "", nil))
}

// TestFsQueriesStore_SaveQueriesTree_SkipsNilEntries proves a nil item in
// Items or a nil sub-folder in Folders is skipped rather than dereferenced -
// defensive slots a caller could leave unset without meaning to save
// anything there.
func TestFsQueriesStore_SaveQueriesTree_SkipsNilEntries(t *testing.T) {
	store := newFsQueriesStore(t.TempDir())
	ctx := context.Background()

	tree := &datatug.QueriesFolder{
		Items: datatug.QueryDefs{
			nil,
			{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q1", Title: "Q1"}},
				Type:        datatug.QueryTypeSQL,
				Text:        "SELECT 1",
			},
		},
		Folders: datatug.QueryFolders{
			nil,
			{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customers"}},
				Items: datatug.QueryDefs{
					{
						ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q2", Title: "Q2"}},
						Type:        datatug.QueryTypeSQL,
						Text:        "SELECT 2",
					},
				},
			},
		},
	}

	require.NoError(t, store.saveQueriesTree(ctx, "", tree))

	loaded, err := store.loadQueriesTree(ctx, "")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Len(t, loaded.Items, 1)
	assert.Equal(t, "q1", loaded.Items[0].ID)
	require.Len(t, loaded.Folders, 1)
	assert.Equal(t, "customers", loaded.Folders[0].ID)
}

// TestFsQueriesStore_SaveQueriesTree_PropagatesItemError proves a query that
// fails validation (here: no Type, which QueryDef.Validate requires) makes
// saveQueriesTree return an error rather than silently continuing.
func TestFsQueriesStore_SaveQueriesTree_PropagatesItemError(t *testing.T) {
	store := newFsQueriesStore(t.TempDir())
	ctx := context.Background()

	tree := &datatug.QueriesFolder{
		Items: datatug.QueryDefs{
			{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "invalid"}}},
		},
	}

	err := store.saveQueriesTree(ctx, "", tree)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "invalid")
}

// TestFsQueriesStore_SaveQueriesTree_PropagatesNestedFolderError proves a
// failure inside a sub-folder propagates all the way up through the
// recursive call, not just the top-level Items loop.
func TestFsQueriesStore_SaveQueriesTree_PropagatesNestedFolderError(t *testing.T) {
	store := newFsQueriesStore(t.TempDir())
	ctx := context.Background()

	tree := &datatug.QueriesFolder{
		Folders: datatug.QueryFolders{
			{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customers"}},
				Items: datatug.QueryDefs{
					{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "invalid"}}},
				},
			},
		},
	}

	err := store.saveQueriesTree(ctx, "", tree)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "invalid")
}
