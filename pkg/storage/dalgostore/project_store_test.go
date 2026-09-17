package dalgostore_test

import (
	"context"
	"testing"
	"time"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage/dalgostore"
)

// newTestDB returns an in-memory dal.DB test double. dalgo2memory is a
// subpackage of the already-required dal-go/dalgo module, not a DALgo
// driver, so using it here does not add a driver dependency to this
// package's non-test import graph (see the deps_test.go check).
func newTestDB() dal.DB {
	return dalgo2memory.New(dalgo2memory.FirestoreProfile())
}

func TestProjectStore_SaveAndLoadProjectFile_RoundTrip(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	store := dalgostore.NewProjectStore(db, "p1")

	created := &datatug.ProjectCreated{At: time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)}
	project := datatug.NewProjectWithStore("p1", store)
	project.Title = "Project One"
	project.Access = "private"
	project.Created = created

	require.NoError(t, store.SaveProject(ctx, project))

	got, err := store.LoadProjectFile(ctx)
	require.NoError(t, err)

	assert.Equal(t, "p1", got.ID)
	assert.Equal(t, "Project One", got.Title)
	assert.Equal(t, "private", got.Access)
	require.NotNil(t, got.Created)
	assert.True(t, created.At.Equal(got.Created.At))
}

func TestProjectStore_SaveProject_WritesTheCanonicalKeyPath(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	store := dalgostore.NewProjectStore(db, "p1")

	project := datatug.NewProjectWithStore("p1", store)
	project.Access = "public"
	project.Created = &datatug.ProjectCreated{At: time.Now()}
	require.NoError(t, store.SaveProject(ctx, project))

	// The canonical key path (REQ:extension-namespace,
	// REQ:hierarchy-is-a-key-path) is built independently of the package
	// under test, from the same dal-go/record primitives the brief
	// specifies (record.NewKeyWithID and a parent key), to prove the
	// project record actually landed at ext/datatug/projects/p1 rather
	// than merely round-tripping through whatever key the package chose.
	extKey := record.NewKeyWithID("ext", "datatug")
	projectKey := record.NewKeyWithParentAndID(extKey, "projects", "p1")
	require.Equal(t, "ext/datatug/projects/p1", projectKey.String())

	var file datatug.ProjectFile
	rec := record.NewRecordWithData(projectKey, &file)
	require.NoError(t, db.Get(ctx, rec))
	assert.Equal(t, "public", file.Access)
}

func TestProjectStore_LoadProjectFile_NotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	store := dalgostore.NewProjectStore(db, "does-not-exist")

	_, err := store.LoadProjectFile(ctx)
	require.Error(t, err)
	assert.True(t, datatug.ProjectDoesNotExist(err), "expected a wrapped datatug.ErrProjectDoesNotExist, got: %v", err)
}

func TestProjectStore_LoadProjectFile_GenericLoadError(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	store := dalgostore.NewProjectStore(db, "p1")

	// Write a JSON object at the project's own key path whose "created"
	// field cannot decode into *datatug.ProjectCreated, forcing Get to fail
	// with something other than record.IsNotFound so LoadProjectFile's
	// generic-error branch runs.
	key := record.NewKeyWithParentAndID(record.NewKeyWithID("ext", "datatug"), "projects", "p1")
	badRecord := map[string]any{"access": "public", "created": "not-an-object"}
	require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, record.NewRecordWithData(key, badRecord))
	}))

	_, err := store.LoadProjectFile(ctx)
	require.Error(t, err)
	assert.False(t, datatug.ProjectDoesNotExist(err))
}

func TestProjectStore_LoadProject_RoundTrip(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	store := dalgostore.NewProjectStore(db, "p1")

	project := datatug.NewProjectWithStore("p1", store)
	project.Title = "Project One"
	project.Access = "protected"
	project.Created = &datatug.ProjectCreated{At: time.Now()}
	require.NoError(t, store.SaveProject(ctx, project))

	loaded, err := store.LoadProject(ctx)
	require.NoError(t, err)
	assert.Equal(t, "p1", loaded.ID)
	assert.Equal(t, "Project One", loaded.Title)
	assert.Equal(t, "protected", loaded.Access)
	require.NotNil(t, loaded.Created)
}

func TestProjectStore_LoadProject_PropagatesLoadProjectFileError(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	store := dalgostore.NewProjectStore(db, "does-not-exist")

	_, err := store.LoadProject(ctx)
	require.Error(t, err)
	assert.True(t, datatug.ProjectDoesNotExist(err))
}

func TestProjectStore_SaveProject_TwoProjectsDoNotCollide(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	store1 := dalgostore.NewProjectStore(db, "p1")
	store2 := dalgostore.NewProjectStore(db, "p2")

	require.NoError(t, store1.SaveProject(ctx, projectWithAccess(store1, "p1", "private")))
	require.NoError(t, store2.SaveProject(ctx, projectWithAccess(store2, "p2", "public")))

	f1, err := store1.LoadProjectFile(ctx)
	require.NoError(t, err)
	f2, err := store2.LoadProjectFile(ctx)
	require.NoError(t, err)

	assert.Equal(t, "private", f1.Access)
	assert.Equal(t, "public", f2.Access)
	assert.Equal(t, "p1", f1.ID)
	assert.Equal(t, "p2", f2.ID)
}

func TestProjectStore_SaveProject_InvalidProjectIsRejected(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	store := dalgostore.NewProjectStore(db, "p1")

	project := datatug.NewProjectWithStore("p1", store)
	// Access is left empty, which ProjectFile.Validate rejects.
	project.Created = &datatug.ProjectCreated{At: time.Now()}

	err := store.SaveProject(ctx, project)
	require.Error(t, err)

	_, loadErr := store.LoadProjectFile(ctx)
	require.Error(t, loadErr)
	assert.True(t, datatug.ProjectDoesNotExist(loadErr), "an invalid project must write nothing")
}

func TestNewProjectStore_PanicsWithoutDB(t *testing.T) {
	assert.Panics(t, func() {
		dalgostore.NewProjectStore(nil, "p1")
	})
}

func TestNewProjectStore_PanicsWithoutProjectID(t *testing.T) {
	assert.Panics(t, func() {
		dalgostore.NewProjectStore(newTestDB(), "")
	})
}

func TestProjectID(t *testing.T) {
	store := dalgostore.NewProjectStore(newTestDB(), "p1")
	assert.Equal(t, "p1", store.ProjectID())
}

func projectWithAccess(store datatug.ProjectStore, id, access string) *datatug.Project {
	p := datatug.NewProjectWithStore(id, store)
	p.Access = access
	p.Created = &datatug.ProjectCreated{At: time.Now()}
	return p
}
