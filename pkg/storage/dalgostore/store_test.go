package dalgostore_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/strongo/validation"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/datatug/datatug-core/pkg/storage/dalgostore"
)

const testStoreID = "s1"

// newTestStore returns a Store over an in-memory dal.DB. The store touches
// no file system, so nothing else is needed to exercise it.
func newTestStore(t *testing.T) (*dalgostore.Store, dal.DB) {
	t.Helper()
	db := newTestDB()
	return dalgostore.NewStore(db, testStoreID), db
}

// createRequest builds a valid create-project request for this store.
func createRequest(id, title string) dto.CreateProjectRequest {
	return dto.CreateProjectRequest{StoreID: testStoreID, ID: id, Title: title}
}

func TestNewStore_ImplementsStorageStore(t *testing.T) {
	var s storage.Store = dalgostore.NewStore(newTestDB(), testStoreID)
	require.NotNil(t, s)
	assert.Equal(t, testStoreID, s.(*dalgostore.Store).ID())
}

func TestStore_GetProjectStore(t *testing.T) {
	store, _ := newTestStore(t)
	projectStore := store.GetProjectStore("p1")
	require.NotNil(t, projectStore)
	assert.Equal(t, "p1", projectStore.ProjectID())
	// The project need not exist for GetProjectStore to return a store,
	// exactly as in filestore.
	_, err := projectStore.LoadProjectFile(context.Background())
	assert.True(t, datatug.ProjectDoesNotExist(err), "unexpected error: %v", err)
}

func TestStore_CreateProject(t *testing.T) {
	ctx := context.Background()

	t.Run("creates the project record under the caller's id", func(t *testing.T) {
		store, db := newTestStore(t)

		summary, err := store.CreateProject(ctx, createRequest("my-first-project", "My First Project"))
		require.NoError(t, err)
		require.NotNil(t, summary)
		assert.Equal(t, "my-first-project", summary.ID)
		assert.Equal(t, "My First Project", summary.Title)
		assert.Equal(t, "private", summary.Access)
		require.NotNil(t, summary.Created)
		assert.False(t, summary.Created.At.IsZero())

		// The record lands at the canonical key path, built here from the
		// same dal-go/record primitives, independently of the package.
		extKey := record.NewKeyWithID("ext", "datatug")
		key := record.NewKeyWithParentAndID(extKey, "projects", "my-first-project")
		var file datatug.ProjectFile
		require.NoError(t, db.Get(ctx, record.NewRecordWithData(key, &file)))
		assert.Equal(t, "My First Project", file.Title)
		assert.Equal(t, "private", file.Access)

		// ...and is readable through the project store for that project.
		loaded, err := store.GetProjectStore("my-first-project").LoadProjectFile(ctx)
		require.NoError(t, err)
		assert.Equal(t, "my-first-project", loaded.ID)
		assert.Equal(t, "My First Project", loaded.Title)
	})

	t.Run("creates several projects in one store", func(t *testing.T) {
		store, _ := newTestStore(t)
		_, err := store.CreateProject(ctx, createRequest("one", "One"))
		require.NoError(t, err)
		_, err = store.CreateProject(ctx, createRequest("two", "Two"))
		require.NoError(t, err)
	})

	t.Run("refuses a project whose id is already taken, naming the id", func(t *testing.T) {
		store, _ := newTestStore(t)
		_, err := store.CreateProject(ctx, createRequest("taken", "First"))
		require.NoError(t, err)

		summary, err := store.CreateProject(ctx, createRequest("taken", "Second"))
		require.Error(t, err)
		assert.Nil(t, summary)
		assert.True(t, record.IsAlreadyExists(err), "expected a record-exists error, got: %v", err)
		assert.Contains(t, err.Error(), `project "taken" already exists`)

		// The project that holds the id is untouched by the refusal.
		loaded, err := store.GetProjectStore("taken").LoadProjectFile(ctx)
		require.NoError(t, err)
		assert.Equal(t, "First", loaded.Title)
	})

	t.Run("refuses an invalid request", func(t *testing.T) {
		store, _ := newTestStore(t)
		for name, request := range map[string]dto.CreateProjectRequest{
			"no title": {StoreID: testStoreID, ID: "p1"},
			"no store": {ID: "p1", Title: "T"},
			"no id":    {StoreID: testStoreID, Title: "T"},
		} {
			t.Run(name, func(t *testing.T) {
				_, err := store.CreateProject(ctx, request)
				require.Error(t, err)
				assert.True(t, validation.IsBadRequestError(err), "unexpected error: %v", err)
			})
		}
	})

	t.Run("refuses an id that breaks the project-id rules", func(t *testing.T) {
		store, _ := newTestStore(t)
		for _, id := range []string{"Upper", "with space", "a/b", "../escape", "-lead", "trail-", "_lead", "tail_", "dot.ted", "тест"} {
			t.Run(id, func(t *testing.T) {
				_, err := store.CreateProject(ctx, createRequest(id, "T"))
				require.Error(t, err)
				var badField validation.ErrBadFieldValue
				require.True(t, errors.As(err, &badField), "unexpected error: %v", err)
				assert.Equal(t, "id", badField.Field)
			})
		}
	})

	t.Run("refuses a request addressed to another store", func(t *testing.T) {
		store, _ := newTestStore(t)
		_, err := store.CreateProject(ctx, dto.CreateProjectRequest{StoreID: "other", ID: "p1", Title: "T"})
		require.Error(t, err)
		var badField validation.ErrBadFieldValue
		require.True(t, errors.As(err, &badField), "unexpected error: %v", err)
		assert.Equal(t, "store", badField.Field)
	})
}

func TestStore_DeleteProject(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes the project record", func(t *testing.T) {
		store, _ := newTestStore(t)
		_, err := store.CreateProject(ctx, createRequest("doomed", "Doomed"))
		require.NoError(t, err)

		require.NoError(t, store.DeleteProject(ctx, "doomed"))

		_, err = store.GetProjectStore("doomed").LoadProjectFile(ctx)
		assert.True(t, datatug.ProjectDoesNotExist(err), "unexpected error: %v", err)

		projects, err := store.GetProjects(ctx)
		require.NoError(t, err)
		assert.Empty(t, projects)
	})

	t.Run("deleting a project that does not exist is not an error", func(t *testing.T) {
		store, _ := newTestStore(t)
		assert.NoError(t, store.DeleteProject(ctx, "never-existed"))
	})

	t.Run("refuses an empty project id", func(t *testing.T) {
		store, _ := newTestStore(t)
		err := store.DeleteProject(ctx, "  ")
		require.Error(t, err)
		assert.True(t, validation.IsBadRequestError(err), "unexpected error: %v", err)
	})
}

func TestStore_GetProjects(t *testing.T) {
	ctx := context.Background()

	t.Run("returns a brief per project", func(t *testing.T) {
		store, _ := newTestStore(t)
		for id, title := range map[string]string{"alpha": "Alpha Project", "beta": "Beta Project"} {
			_, err := store.CreateProject(ctx, createRequest(id, title))
			require.NoError(t, err)
		}

		projects, err := store.GetProjects(ctx)
		require.NoError(t, err)
		require.Len(t, projects, 2)
		sort.Slice(projects, func(i, j int) bool { return projects[i].ID < projects[j].ID })

		assert.Equal(t, "alpha", projects[0].ID)
		assert.Equal(t, "Alpha Project", projects[0].Title)
		assert.Equal(t, "private", projects[0].Access)
		assert.Equal(t, "beta", projects[1].ID)
		assert.Equal(t, "Beta Project", projects[1].Title)

		// Every brief is valid on its own terms.
		for i := range projects {
			assert.NoError(t, projects[i].Validate())
		}
	})

	t.Run("the key is the authority for the id, not the stored payload", func(t *testing.T) {
		store, db := newTestStore(t)
		// A record whose stored id disagrees with the key it lives under —
		// what a hand-edited file or an older writer can leave behind. The
		// key wins, in GetProjects and in LoadProjectFile alike.
		key := record.NewKeyWithParentAndID(record.NewKeyWithID("ext", "datatug"), "projects", "key-id")
		file := datatug.ProjectFile{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{ID: "payload-id", Title: "Mismatched"},
				Access:        "private",
			},
			Created: &datatug.ProjectCreated{At: time.Now().UTC()},
		}
		require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			return tx.Set(ctx, record.NewRecordWithData(key, &file))
		}))

		projects, err := store.GetProjects(ctx)
		require.NoError(t, err)
		require.Len(t, projects, 1)
		assert.Equal(t, "key-id", projects[0].ID, "the key's id must win over the stored one")
		assert.Equal(t, "Mismatched", projects[0].Title)

		loaded, err := store.GetProjectStore("key-id").LoadProjectFile(ctx)
		require.NoError(t, err)
		assert.Equal(t, "key-id", loaded.ID, "the key's id must win over the stored one")
	})

	t.Run("returns the repository of a project that has one", func(t *testing.T) {
		store, _ := newTestStore(t)
		projectStore := store.GetProjectStore("p1")
		project := datatug.NewProjectWithStore("p1", projectStore)
		project.Access = "public"
		project.Created = &datatug.ProjectCreated{At: time.Now()}
		project.Repository = &datatug.ProjectRepository{Type: "git", WebURL: "https://example.com/p1"}
		require.NoError(t, projectStore.SaveProject(ctx, project))

		projects, err := store.GetProjects(ctx)
		require.NoError(t, err)
		require.Len(t, projects, 1)
		assert.Equal(t, "p1", projects[0].ID)
		assert.Equal(t, "public", projects[0].Access)
		require.NotNil(t, projects[0].Repository)
		assert.Equal(t, "https://example.com/p1", projects[0].Repository.WebURL)
	})

	t.Run("returns nothing for an empty store", func(t *testing.T) {
		store, _ := newTestStore(t)
		projects, err := store.GetProjects(ctx)
		require.NoError(t, err)
		assert.Empty(t, projects)
	})
}

// TestStore_TitleSurvivesAnUnmodifiedSaveRoundTrip is the regression test
// for the defect filestore still has: create -> LoadProject -> SaveProject
// with nothing changed must leave the title in place, and the brief the
// project then lists under must still pass its own validation. A
// SaveProject that dropped the title would wipe it on any no-op save and
// produce briefs that fail ProjectBrief.Validate().
func TestStore_TitleSurvivesAnUnmodifiedSaveRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestStore(t)

	_, err := store.CreateProject(ctx, createRequest("round-trip", "Round Trip"))
	require.NoError(t, err)

	projectStore := store.GetProjectStore("round-trip")
	project, err := projectStore.LoadProject(ctx, datatug.Depth(1))
	require.NoError(t, err)
	require.Equal(t, "Round Trip", project.Title, "the loaded project must carry its title")

	// Saved back unmodified.
	require.NoError(t, projectStore.SaveProject(ctx, project))

	projects, err := store.GetProjects(ctx)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, "round-trip", projects[0].ID)
	assert.Equal(t, "Round Trip", projects[0].Title, "an unmodified save must not wipe the title")
	assert.NoError(t, projects[0].Validate())
}
