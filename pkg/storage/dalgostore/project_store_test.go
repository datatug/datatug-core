package dalgostore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/strongo/validation"

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
	// The title MUST round-trip: GetProjects reads a project's title back
	// out of the project record, so a save that dropped it would wipe the
	// title on every save, including one that changed nothing else.
	project.Title = "Project One"
	project.Access = "private"
	project.Created = created

	require.NoError(t, store.SaveProject(ctx, project))

	got, err := store.LoadProjectFile(ctx)
	require.NoError(t, err)

	assert.Equal(t, "p1", got.ID)
	assert.Equal(t, "Project One", got.Title, "the title must survive a save round-trip")
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

func TestProjectStore_SaveProject_NeverWritesTheExtensionScopingRecord(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	store := dalgostore.NewProjectStore(db, "p1")

	require.NoError(t, store.SaveProject(ctx, projectWithAccess(store, "p1", "public")))

	// REQ:extension-namespace: ext/datatug is a scoping parent only. It need
	// not exist as a record of its own, exactly as a Firestore parent
	// document need not exist.
	extKey := record.NewKeyWithID("ext", "datatug")
	exists, err := db.Exists(ctx, extKey)
	require.NoError(t, err)
	assert.False(t, exists, "ext/datatug must never be written as a record")
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
	project.Access = "protected"
	project.Created = &datatug.ProjectCreated{At: time.Now()}
	require.NoError(t, store.SaveProject(ctx, project))

	// datatug.Depth(1) asks for just the project record; see LoadProject's
	// doc comment for why the default (no options) is not implemented.
	loaded, err := store.LoadProject(ctx, datatug.Depth(1))
	require.NoError(t, err)
	assert.Equal(t, "p1", loaded.ID)
	assert.Equal(t, "protected", loaded.Access)
	require.NotNil(t, loaded.Created)
}

func TestProjectStore_LoadProject_DefaultDepthIsNotImplemented(t *testing.T) {
	ctx := context.Background()
	store := dalgostore.NewProjectStore(newTestDB(), "p1")

	// No options given: Depth() is 0, filestore's own "load everything"
	// signal (pkg/storage/filestore/store_loader.go:27). This package does
	// not load the full project graph yet.
	_, err := store.LoadProject(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, dalgostore.ErrNotImplemented))
}

func TestProjectStore_LoadProject_DeeperThanOneIsNotImplemented(t *testing.T) {
	ctx := context.Background()
	store := dalgostore.NewProjectStore(newTestDB(), "p1")

	_, err := store.LoadProject(ctx, datatug.Depth(2))
	require.Error(t, err)
	assert.True(t, errors.Is(err, dalgostore.ErrNotImplemented))
}

func TestProjectStore_LoadProject_PropagatesLoadProjectFileError(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	store := dalgostore.NewProjectStore(db, "does-not-exist")

	_, err := store.LoadProject(ctx, datatug.Depth(1))
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
	// Access is left empty, which Project.Validate (and ProjectFile.Validate)
	// rejects.
	project.Created = &datatug.ProjectCreated{At: time.Now()}

	err := store.SaveProject(ctx, project)
	require.Error(t, err)

	_, loadErr := store.LoadProjectFile(ctx)
	require.Error(t, loadErr)
	assert.True(t, datatug.ProjectDoesNotExist(loadErr), "an invalid project must write nothing")
}

// TestProjectStore_SaveProject_MissingCreatedIsRejectedByFileValidate proves
// SaveProject's second validation — the assembled ProjectFile's own
// Validate, mirroring filestore's putProjectFile
// (pkg/storage/filestore/store_project_saver.go:113-115) — actually runs and
// is not a no-op duplicate of Project.Validate: Project.Validate never
// checks Created (pkg/datatug/project.go:95-151), so a project with a valid
// Access but no Created passes Project.Validate and is only caught by
// ProjectFile.Validate.
func TestProjectStore_SaveProject_MissingCreatedIsRejectedByFileValidate(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	store := dalgostore.NewProjectStore(db, "p1")

	project := datatug.NewProjectWithStore("p1", store)
	project.Access = "public"
	// Created is deliberately left nil.

	err := store.SaveProject(ctx, project)
	require.Error(t, err)

	_, loadErr := store.LoadProjectFile(ctx)
	require.Error(t, loadErr)
	assert.True(t, datatug.ProjectDoesNotExist(loadErr), "a project missing Created must write nothing")
}

func TestProjectStore_SaveProject_ProjectIDMismatchIsRejected(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	store := dalgostore.NewProjectStore(db, "p1")

	project := projectWithAccess(store, "some-other-id", "public")

	err := store.SaveProject(ctx, project)
	require.Error(t, err)
	assert.True(t, validation.IsBadFieldValueError(err), "expected a typed validation.ErrBadFieldValue, got: %v", err)

	_, loadErr := store.LoadProjectFile(ctx)
	require.Error(t, loadErr)
	assert.True(t, datatug.ProjectDoesNotExist(loadErr), "a project-id mismatch must write nothing")
}

// TestProjectStore_SaveProject_UnsupportedDataIsRejected proves that a
// project carrying data in a collection this store does not persist yet is
// rejected with ErrNotImplemented and writes nothing, rather than silently
// dropping that data (dalgo-project-store plan tasks 8-11 add these
// collections one at a time).
func TestProjectStore_SaveProject_UnsupportedDataIsRejected(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(p *datatug.Project)
	}{
		{"queries", func(p *datatug.Project) {
			p.Queries = &datatug.QueriesFolder{Items: datatug.QueryDefs{{}}}
		}},
		{"boards", func(p *datatug.Project) {
			p.Boards = datatug.Boards{{}}
		}},
		{"entities", func(p *datatug.Project) {
			p.Entities = datatug.Entities{{}}
		}},
		{"environments", func(p *datatug.Project) {
			p.Environments = datatug.Environments{{}}
		}},
		{"db models", func(p *datatug.Project) {
			p.DbModels = datatug.DbModels{{}}
		}},
		{"db drivers", func(p *datatug.Project) {
			p.DbDrivers = datatug.ProjDbDrivers{{}}
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			db := newTestDB()
			store := dalgostore.NewProjectStore(db, "p1")
			project := projectWithAccess(store, "p1", "public")
			c.mutate(project)

			err := store.SaveProject(ctx, project)
			require.Error(t, err)
			assert.True(t, errors.Is(err, dalgostore.ErrNotImplemented), "%s: expected ErrNotImplemented, got: %v", c.name, err)

			_, loadErr := store.LoadProjectFile(ctx)
			require.Error(t, loadErr)
			assert.True(t, datatug.ProjectDoesNotExist(loadErr), "%s: unsupported data must write nothing", c.name)
		})
	}
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
