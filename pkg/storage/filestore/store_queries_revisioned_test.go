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

// TestPutQuery_AcceptsGraphQL is the end-to-end regression test for S6:
// legacy SaveQuery has always accepted a "GraphQL"-typed query (it only
// calls QueryDef.Validate(), which explicitly allows it); PutQuery must
// accept it too.
func TestPutQuery_AcceptsGraphQL(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	q := datatug.QueryDefWithFolderPath{
		QueryDef: datatug.QueryDef{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q1", Title: "Q1"}},
			Type:        "GraphQL",
			Text:        "query { customers { id } }",
		},
	}
	stored, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatalf("expected a GraphQL query to be accepted, matching legacy SaveQuery, got: %v", err)
	}
	if stored.Revision == "" {
		t.Fatal("expected a non-empty revision")
	}
	if !fileExists(t, filepath.Join(queriesDir, "q1.query.graphql")) {
		t.Error("expected q1.query.graphql to exist")
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
	var incomplete *datatug.IncompleteQueryRecordError
	if !errors.As(err, &incomplete) {
		t.Fatalf("expected an IncompleteQueryRecordError, got %T: %v", err, err)
	}
	// S1: the error must expose the incomplete record - a revision computed
	// over exactly what is currently persisted, and the partial record
	// itself - not just the fact that something is wrong. Without this, a
	// caller has no way to ever act on this location again (see the tests
	// below).
	if incomplete.Revision == "" {
		t.Fatal("expected the incomplete record error to carry a non-empty revision")
	}
	if incomplete.Query.ID != "q1" || incomplete.Query.Type != datatug.QueryTypeDTQL {
		t.Fatalf("expected the incomplete record error to carry the partial record, got: %+v", incomplete.Query)
	}
	if incomplete.Query.Text != "" {
		t.Fatalf("expected the partial record's Text to be empty (no body sidecar exists), got %q", incomplete.Query.Text)
	}
}

// --- S1: a legacy body-less ".query.json" is not a permanent dead end ---
//
// A record whose JSON metadata exists but whose expected body sidecar does
// not - the documented normal shape for "a query with only structured
// parameters/recordsets so far" (store_queries.go's readQueryTextSidecar
// doc comment), or simply a hand-placed/very old file - used to be
// unreachable by every revisioned write: IfNoneMatch correctly refused it
// (something exists), but there was no IfMatch value LoadQueryRevision
// could ever hand back (it only ever returned an error), so no PutQuery or
// DeleteQueryRevision call could ever touch it. These tests prove all four
// paths now work, without any data loss: the record's identity is never
// guessed at or discarded, only ever replaced by a caller that already
// proved (via IfMatch) it saw the exact incomplete state first.

func writeIncompleteLegacyQuery(t *testing.T, queriesDir, id string) {
	t.Helper()
	if err := os.MkdirAll(queriesDir, 0o777); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(queriesDir, id+".query.json"),
		[]byte(`{"id":"`+id+`","title":"Legacy","type":"SQL"}`), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestPutQuery_TreatsLegacyEmptyBodyRecordAsIncomplete is the currentQueryPair
// design decision query_txn.go's doc comment points to: a legacy record with
// a JSON sidecar but no body is never "complete", so an IfNoneMatch create
// still refuses (something exists at this location) - it never silently
// treats "no usable content" as "nothing here".
func TestPutQuery_TreatsLegacyEmptyBodyRecordAsIncomplete(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	writeIncompleteLegacyQuery(t, queriesDir, "legacy1")

	q := datatug.QueryDefWithFolderPath{
		QueryDef: datatug.QueryDef{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "legacy1", Title: "Legacy1"}},
			Type:        datatug.QueryTypeSQL,
			Text:        "SELECT 1",
		},
	}
	_, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if !datatug.IsQueryRevisionConflict(err) {
		t.Fatalf("expected a QueryRevisionConflictError for IfNoneMatch over an incomplete record, got %T: %v", err, err)
	}
	// An arbitrary IfMatch guess must not match either.
	_, err = store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfMatch: "not-the-real-revision"})
	if !datatug.IsQueryRevisionConflict(err) {
		t.Fatalf("expected a QueryRevisionConflictError for a wrong IfMatch over an incomplete record, got %T: %v", err, err)
	}
}

// TestPutQuery_IfMatchWithIncompleteRevisionCompletesTheRecord proves the
// actual fix: the revision LoadQueryRevision's IncompleteQueryRecordError
// carries is exactly the value a caller needs to pass as IfMatch to
// replace - and thereby complete - the record.
func TestPutQuery_IfMatchWithIncompleteRevisionCompletesTheRecord(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	writeIncompleteLegacyQuery(t, queriesDir, "legacy1")
	ctx := context.Background()

	_, err := store.LoadQueryRevision(ctx, "legacy1")
	var incomplete *datatug.IncompleteQueryRecordError
	if !errors.As(err, &incomplete) {
		t.Fatalf("expected an IncompleteQueryRecordError, got %T: %v", err, err)
	}

	q := datatug.QueryDefWithFolderPath{
		QueryDef: datatug.QueryDef{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "legacy1", Title: "Legacy1"}},
			Type:        datatug.QueryTypeSQL,
			Text:        "SELECT 1",
		},
	}
	stored, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfMatch: incomplete.Revision})
	if err != nil {
		t.Fatalf("expected PutQuery(IfMatch: <incomplete revision>) to complete the record, got: %v", err)
	}
	if stored.Revision == incomplete.Revision {
		t.Fatal("expected a new revision once the record is complete")
	}
	if !fileExists(t, filepath.Join(queriesDir, "legacy1.query.sql")) {
		t.Error("expected the body sidecar to now exist")
	}

	loaded, err := store.LoadQueryRevision(ctx, "legacy1")
	if err != nil {
		t.Fatalf("unexpected error loading the now-complete record: %v", err)
	}
	if loaded.Query.Text != "SELECT 1" || loaded.Revision != stored.Revision {
		t.Fatalf("expected the completed record to round-trip, got: %+v", loaded)
	}
}

// TestDeleteQueryRevision_IfMatchWithIncompleteRevisionDeletesTheRecord
// proves the same reachability for delete: a caller that saw the exact
// incomplete state via LoadQueryRevision can remove it.
func TestDeleteQueryRevision_IfMatchWithIncompleteRevisionDeletesTheRecord(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	writeIncompleteLegacyQuery(t, queriesDir, "legacy1")
	ctx := context.Background()

	_, err := store.LoadQueryRevision(ctx, "legacy1")
	var incomplete *datatug.IncompleteQueryRecordError
	if !errors.As(err, &incomplete) {
		t.Fatalf("expected an IncompleteQueryRecordError, got %T: %v", err, err)
	}

	if err := store.DeleteQueryRevision(ctx, "legacy1", incomplete.Revision); err != nil {
		t.Fatalf("expected DeleteQueryRevision(<incomplete revision>) to succeed, got: %v", err)
	}
	if fileExists(t, filepath.Join(queriesDir, "legacy1.query.json")) {
		t.Error("expected the JSON metadata to be removed")
	}
}

// TestDeleteQueryRevision_WrongRevisionStillConflictsOnIncompleteRecord
// proves the delete precondition is still enforced, not bypassed, for an
// incomplete record.
func TestDeleteQueryRevision_WrongRevisionStillConflictsOnIncompleteRecord(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	writeIncompleteLegacyQuery(t, queriesDir, "legacy1")

	err := store.DeleteQueryRevision(context.Background(), "legacy1", "not-the-real-revision")
	if !datatug.IsQueryRevisionConflict(err) {
		t.Fatalf("expected a QueryRevisionConflictError, got %T: %v", err, err)
	}
	if !fileExists(t, filepath.Join(queriesDir, "legacy1.query.json")) {
		t.Error("expected the JSON metadata to remain after a rejected delete")
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
