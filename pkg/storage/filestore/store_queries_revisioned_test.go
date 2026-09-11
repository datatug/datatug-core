package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

func newTestQueriesStore(t *testing.T) (fsQueriesStore, string) {
	t.Helper()
	projectDir := t.TempDir()
	return newFsQueriesStore(projectDir), filepath.Join(projectDir, storage.QueriesFolder)
}

func dtqlQuery(id, folderPath, text string) datatug.QueryDefWithFolderPath {
	return datatug.QueryDefWithFolderPath{
		FolderPath: folderPath,
		QueryDef: datatug.QueryDef{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: id, Title: "Title " + id}},
			Type:        datatug.QueryTypeDTQL,
			Text:        text,
		},
	}
}

// --- create (IfNoneMatch) ---

func TestPutQuery_CreateSucceeds(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "from:\n  name: Invoice\n")

	stored, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored.Revision == "" {
		t.Fatal("expected a non-empty revision")
	}
	if !fileExists(t, filepath.Join(queriesDir, "q1.query.json")) {
		t.Error("expected q1.query.json to exist")
	}
	if !fileExists(t, filepath.Join(queriesDir, "q1.query.dtql")) {
		t.Error("expected q1.query.dtql to exist")
	}
}

func TestPutQuery_CreateOverExistingConflicts(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "original")
	if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(queriesDir, "q1.query.dtql"))
	if err != nil {
		t.Fatalf("unexpected error reading body: %v", err)
	}

	q2 := dtqlQuery("q1", "", "different content")
	_, err = store.PutQuery(ctx, &q2, datatug.QueryWriteCondition{IfNoneMatch: true})
	if !datatug.IsQueryRevisionConflict(err) {
		t.Fatalf("expected a QueryRevisionConflictError, got %T: %v", err, err)
	}
	after, err := os.ReadFile(filepath.Join(queriesDir, "q1.query.dtql"))
	if err != nil {
		t.Fatalf("unexpected error reading body: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("expected the original body to be unchanged, got %q", after)
	}
}

// --- update (IfMatch) ---

func TestPutQuery_UpdateWithCurrentRevisionSucceeds(t *testing.T) {
	store, _ := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "original")
	stored, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	updated := dtqlQuery("q1", "", "updated")
	stored2, err := store.PutQuery(ctx, &updated, datatug.QueryWriteCondition{IfMatch: stored.Revision})
	if err != nil {
		t.Fatalf("unexpected error updating: %v", err)
	}
	if stored2.Revision == stored.Revision {
		t.Fatal("expected the revision to change after an update")
	}

	loaded, err := store.LoadQueryRevision(ctx, "q1")
	if err != nil {
		t.Fatalf("unexpected error loading: %v", err)
	}
	if loaded.Query.Text != "updated" {
		t.Fatalf("expected updated text, got %q", loaded.Query.Text)
	}
	if loaded.Revision != stored2.Revision {
		t.Fatalf("expected loaded revision to equal the revision PutQuery returned")
	}
}

func TestPutQuery_UpdateWithStaleRevisionConflicts(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "original")
	if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	updated := dtqlQuery("q1", "", "updated")
	_, err := store.PutQuery(ctx, &updated, datatug.QueryWriteCondition{IfMatch: "not-the-real-revision"})
	var conflict *datatug.QueryRevisionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected a QueryRevisionConflictError, got %T: %v", err, err)
	}
	body, readErr := os.ReadFile(filepath.Join(queriesDir, "q1.query.dtql"))
	if readErr != nil {
		t.Fatalf("unexpected error reading body: %v", readErr)
	}
	if string(body) != "original" {
		t.Fatalf("expected the original body to be unchanged, got %q", body)
	}
}

func TestPutQuery_UpdateMissingRecordConflicts(t *testing.T) {
	store, _ := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("does-not-exist", "", "text")
	_, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfMatch: "any-revision"})
	if !datatug.IsQueryRevisionConflict(err) {
		t.Fatalf("expected a QueryRevisionConflictError, got %T: %v", err, err)
	}
}

func TestPutQuery_TypeChangeRemovesOldSidecar(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	sqlQuery := datatug.QueryDefWithFolderPath{
		QueryDef: datatug.QueryDef{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q1", Title: "Q1"}},
			Type:        datatug.QueryTypeSQL,
			Text:        "SELECT 1",
		},
	}
	stored, err := store.PutQuery(ctx, &sqlQuery, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}
	if !fileExists(t, filepath.Join(queriesDir, "q1.query.sql")) {
		t.Fatal("expected q1.query.sql to exist")
	}

	dtqlUpdate := dtqlQuery("q1", "", "from:\n  name: Invoice\n")
	if _, err := store.PutQuery(ctx, &dtqlUpdate, datatug.QueryWriteCondition{IfMatch: stored.Revision}); err != nil {
		t.Fatalf("unexpected error updating: %v", err)
	}
	if fileExists(t, filepath.Join(queriesDir, "q1.query.sql")) {
		t.Error("expected the stale .sql sidecar to be removed on a type change")
	}
	if !fileExists(t, filepath.Join(queriesDir, "q1.query.dtql")) {
		t.Error("expected the new .dtql sidecar to exist")
	}
}

// --- delete ---

func TestDeleteQueryRevision_MatchingRevisionRemovesBothFiles(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "text")
	stored, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	if err := store.DeleteQueryRevision(ctx, "q1", stored.Revision); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}
	if fileExists(t, filepath.Join(queriesDir, "q1.query.json")) {
		t.Error("expected q1.query.json to be removed")
	}
	if fileExists(t, filepath.Join(queriesDir, "q1.query.dtql")) {
		t.Error("expected q1.query.dtql to be removed")
	}
}

func TestDeleteQueryRevision_StaleRevisionConflicts(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "text")
	if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	err := store.DeleteQueryRevision(ctx, "q1", "not-the-real-revision")
	if !datatug.IsQueryRevisionConflict(err) {
		t.Fatalf("expected a QueryRevisionConflictError, got %T: %v", err, err)
	}
	if !fileExists(t, filepath.Join(queriesDir, "q1.query.json")) {
		t.Error("expected q1.query.json to remain after a rejected delete")
	}
	if !fileExists(t, filepath.Join(queriesDir, "q1.query.dtql")) {
		t.Error("expected q1.query.dtql to remain after a rejected delete")
	}
}

func TestDeleteQueryRevision_MissingRecordConflicts(t *testing.T) {
	store, _ := newTestQueriesStore(t)
	ctx := context.Background()
	err := store.DeleteQueryRevision(ctx, "does-not-exist", "any-revision")
	if !datatug.IsQueryRevisionConflict(err) {
		t.Fatalf("expected a QueryRevisionConflictError, got %T: %v", err, err)
	}
}

// --- load ---

func TestLoadQueryRevision_NotFound(t *testing.T) {
	store, _ := newTestQueriesStore(t)
	_, err := store.LoadQueryRevision(context.Background(), "does-not-exist")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected an os.ErrNotExist-wrapping error, got %T: %v", err, err)
	}
}

func TestLoadQueryRevision_IncompleteJSONOnlyRecordRejected(t *testing.T) {
	// A legacy-written record with empty Text has no body sidecar; the
	// design choice documented on currentQueryPair.complete treats that as
	// incomplete from the strict revisioned store's point of view, even
	// though the tolerant legacy LoadQuery accepts it fine (see
	// TestLoadQuery_without_text_sidecar in queries_store_test.go).
	store, queriesDir := newTestQueriesStore(t)
	if err := os.MkdirAll(queriesDir, 0o777); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(queriesDir, "q1.query.json"),
		[]byte(`{"id":"q1","title":"Q1","type":"DTQL"}`), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := store.LoadQueryRevision(context.Background(), "q1")
	if !datatug.IsIncompleteQueryRecord(err) {
		t.Fatalf("expected an IncompleteQueryRecordError, got %T: %v", err, err)
	}
}

func TestPutQuery_RejectsCredentialsBeforeStaging(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	q := datatug.QueryDefWithFolderPath{
		QueryDef: datatug.QueryDef{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q1", Title: "Q1"}},
			Type:        datatug.QueryTypeSQL,
			Text:        "SELECT 1",
			Targets:     []datatug.QueryDefTarget{{Driver: "postgres", Credentials: datatug.Credentials{Username: "a", Password: "secret"}}},
		},
	}
	_, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err == nil {
		t.Fatal("expected an error for a target password")
	}
	if fileExists(t, filepath.Join(queriesDir, "q1.query.json")) {
		t.Error("expected no file to be written for a rejected request")
	}
}

func TestPutQuery_RejectsInvalidLocationBeforeStaging(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	q := dtqlQuery("q1", "../escape", "text")
	_, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if !datatug.IsInvalidQueryLocation(err) {
		t.Fatalf("expected an InvalidQueryLocationError, got %T: %v", err, err)
	}
	if _, statErr := os.Stat(queriesDir); statErr == nil {
		entries, _ := os.ReadDir(queriesDir)
		if len(entries) != 0 {
			t.Errorf("expected no files created, found: %v", entries)
		}
	}
}

func TestPutQuery_InvalidConditionRejectedBeforeStaging(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	q := dtqlQuery("q1", "", "text")
	_, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{})
	if err == nil {
		t.Fatal("expected an error for an invalid condition")
	}
	if _, statErr := os.Stat(queriesDir); !os.IsNotExist(statErr) {
		t.Errorf("expected no queries directory to be created, stat err: %v", statErr)
	}
}

func TestPutQuery_CancelledContextBeforeIOWritesNothing(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	q := dtqlQuery("q1", "", "text")
	_, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %T: %v", err, err)
	}
	if fileExists(t, filepath.Join(queriesDir, "q1.query.json")) {
		t.Error("expected no file to be written when the context is already cancelled")
	}
}

func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	t.Fatalf("unexpected error checking %s: %v", path, err)
	return false
}
