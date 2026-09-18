package dalgostore_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

// newTestStore returns a Store over an in-memory dal.DB whose store root is
// a fresh temp directory, so the schema install CreateProject performs is
// exercised against a real filesystem without touching the repository.
func newTestStore(t *testing.T) (*dalgostore.Store, dal.DB, string) {
	t.Helper()
	db := newTestDB()
	dir := t.TempDir()
	return dalgostore.NewStore(db, testStoreID, dir), db, dir
}

func TestNewStore_ImplementsStorageStore(t *testing.T) {
	var s storage.Store = dalgostore.NewStore(newTestDB(), testStoreID, t.TempDir())
	require.NotNil(t, s)
	assert.Equal(t, testStoreID, s.(*dalgostore.Store).ID())
}

func TestStore_GetProjectStore(t *testing.T) {
	store, _, _ := newTestStore(t)
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

	t.Run("creates the project record, the schema and the summary", func(t *testing.T) {
		store, db, dir := newTestStore(t)

		summary, err := store.CreateProject(ctx, dto.CreateProjectRequest{StoreID: testStoreID, Title: "My First Project"})
		require.NoError(t, err)
		require.NotNil(t, summary)
		assert.Equal(t, "my-first-project", summary.ID)
		assert.Equal(t, "My First Project", summary.Title)
		assert.Equal(t, "private", summary.Access)
		require.NotNil(t, summary.Created)
		assert.False(t, summary.Created.At.IsZero())

		// The static inGitDB schema is installed into the store root.
		assert.FileExists(t, filepath.Join(dir, ".ingitdb", "root-collections.yaml"))

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

	t.Run("is repeatable against a store whose schema is already installed", func(t *testing.T) {
		store, _, _ := newTestStore(t)
		_, err := store.CreateProject(ctx, dto.CreateProjectRequest{StoreID: testStoreID, Title: "One"})
		require.NoError(t, err)
		_, err = store.CreateProject(ctx, dto.CreateProjectRequest{StoreID: testStoreID, Title: "Two"})
		require.NoError(t, err)
	})

	t.Run("refuses a project whose id is already taken", func(t *testing.T) {
		store, _, _ := newTestStore(t)
		_, err := store.CreateProject(ctx, dto.CreateProjectRequest{StoreID: testStoreID, Title: "Same Title"})
		require.NoError(t, err)

		summary, err := store.CreateProject(ctx, dto.CreateProjectRequest{StoreID: testStoreID, Title: "same title"})
		require.Error(t, err)
		assert.Nil(t, summary)
		assert.True(t, record.IsAlreadyExists(err), "expected a record-exists error, got: %v", err)
	})

	t.Run("refuses an invalid request", func(t *testing.T) {
		store, _, _ := newTestStore(t)
		_, err := store.CreateProject(ctx, dto.CreateProjectRequest{StoreID: testStoreID})
		require.Error(t, err)
		assert.True(t, validation.IsBadRequestError(err), "unexpected error: %v", err)

		_, err = store.CreateProject(ctx, dto.CreateProjectRequest{Title: "No Store"})
		require.Error(t, err)
		assert.True(t, validation.IsBadRequestError(err), "unexpected error: %v", err)
	})

	t.Run("refuses a request addressed to another store", func(t *testing.T) {
		store, _, _ := newTestStore(t)
		_, err := store.CreateProject(ctx, dto.CreateProjectRequest{StoreID: "other", Title: "T"})
		require.Error(t, err)
		var badField validation.ErrBadFieldValue
		require.True(t, errors.As(err, &badField), "unexpected error: %v", err)
		assert.Equal(t, "store", badField.Field)
	})

	t.Run("refuses a title no id can be derived from", func(t *testing.T) {
		store, _, _ := newTestStore(t)
		_, err := store.CreateProject(ctx, dto.CreateProjectRequest{StoreID: testStoreID, Title: "-/-"})
		require.Error(t, err)
		var badField validation.ErrBadFieldValue
		require.True(t, errors.As(err, &badField), "unexpected error: %v", err)
		assert.Equal(t, "title", badField.Field)
	})

	t.Run("refuses a store with no store root directory", func(t *testing.T) {
		store := dalgostore.NewStore(newTestDB(), testStoreID, "")
		_, err := store.CreateProject(ctx, dto.CreateProjectRequest{StoreID: testStoreID, Title: "T"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no store root directory")
	})

	t.Run("reports a failed schema install", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "root")
		// A regular file where the store root must be a directory makes
		// WriteSchema fail, and CreateProject must surface that.
		require.NoError(t, os.WriteFile(dir, []byte("not a directory"), 0o600))
		store := dalgostore.NewStore(newTestDB(), testStoreID, dir)
		_, err := store.CreateProject(ctx, dto.CreateProjectRequest{StoreID: testStoreID, Title: "T"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to install inGitDB schema")
	})
}

func TestStore_DeleteProject(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes the project record", func(t *testing.T) {
		store, _, _ := newTestStore(t)
		_, err := store.CreateProject(ctx, dto.CreateProjectRequest{StoreID: testStoreID, Title: "Doomed"})
		require.NoError(t, err)

		require.NoError(t, store.DeleteProject(ctx, "doomed"))

		_, err = store.GetProjectStore("doomed").LoadProjectFile(ctx)
		assert.True(t, datatug.ProjectDoesNotExist(err), "unexpected error: %v", err)

		projects, err := store.GetProjects(ctx)
		require.NoError(t, err)
		assert.Empty(t, projects)
	})

	t.Run("deleting a project that does not exist is not an error", func(t *testing.T) {
		store, _, _ := newTestStore(t)
		assert.NoError(t, store.DeleteProject(ctx, "never-existed"))
	})

	t.Run("refuses an empty project id", func(t *testing.T) {
		store, _, _ := newTestStore(t)
		err := store.DeleteProject(ctx, "  ")
		require.Error(t, err)
		assert.True(t, validation.IsBadRequestError(err), "unexpected error: %v", err)
	})
}

func TestStore_GetProjects(t *testing.T) {
	ctx := context.Background()

	t.Run("returns a brief per project", func(t *testing.T) {
		store, _, _ := newTestStore(t)
		for _, title := range []string{"Alpha Project", "Beta Project"} {
			_, err := store.CreateProject(ctx, dto.CreateProjectRequest{StoreID: testStoreID, Title: title})
			require.NoError(t, err)
		}

		projects, err := store.GetProjects(ctx)
		require.NoError(t, err)
		require.Len(t, projects, 2)
		sort.Slice(projects, func(i, j int) bool { return projects[i].ID < projects[j].ID })

		assert.Equal(t, "alpha-project", projects[0].ID)
		assert.Equal(t, "Alpha Project", projects[0].Title)
		assert.Equal(t, "private", projects[0].Access)
		assert.Equal(t, "beta-project", projects[1].ID)
		assert.Equal(t, "Beta Project", projects[1].Title)

		// Every brief is valid on its own terms.
		for i := range projects {
			assert.NoError(t, projects[i].Validate())
		}
	})

	t.Run("returns the repository of a project that has one", func(t *testing.T) {
		store, db, _ := newTestStore(t)
		projectStore := store.GetProjectStore("p1")
		project := datatug.NewProjectWithStore("p1", projectStore)
		project.Access = "public"
		project.Created = &datatug.ProjectCreated{At: time.Now()}
		project.Repository = &datatug.ProjectRepository{Type: "git", WebURL: "https://example.com/p1"}
		require.NoError(t, projectStore.SaveProject(ctx, project))
		require.NotNil(t, db)

		projects, err := store.GetProjects(ctx)
		require.NoError(t, err)
		require.Len(t, projects, 1)
		assert.Equal(t, "p1", projects[0].ID)
		assert.Equal(t, "public", projects[0].Access)
		require.NotNil(t, projects[0].Repository)
		assert.Equal(t, "https://example.com/p1", projects[0].Repository.WebURL)
	})

	t.Run("returns nothing for an empty store", func(t *testing.T) {
		store, _, _ := newTestStore(t)
		projects, err := store.GetProjects(ctx)
		require.NoError(t, err)
		assert.Empty(t, projects)
	})
}

func TestProjectIDFromTitle(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		title string
		want  string
	}{
		{"Simple", "simple"},
		{"My First Project", "my-first-project"},
		{"  padded  ", "padded"},
		{"Mixed 123 CASE", "mixed-123-case"},
		{"a---b", "a-b"},
		{"тест project", "project"},
	} {
		t.Run(tt.title, func(t *testing.T) {
			store, _, _ := newTestStore(t)
			summary, err := store.CreateProject(ctx, dto.CreateProjectRequest{StoreID: testStoreID, Title: tt.title})
			require.NoError(t, err)
			assert.Equal(t, tt.want, summary.ID)
		})
	}
}
