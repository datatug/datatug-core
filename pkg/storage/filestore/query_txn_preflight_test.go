package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

// Review X1: a writer must never commit a transaction recovery would
// refuse. Before the fix, a put whose target name held a symlink,
// directory, FIFO, over-cap or unreadable file committed its journal and
// only then found the target unusable; every later read and write of the
// project then failed recovery. commitQueryTransaction now runs recovery's
// own target checks (checkQueryTxnTargets) before the commit point, so the
// write is refused and the store stays usable. These tests assert both
// halves for every trigger the review reproduced (the unix-only triggers
// are in query_txn_preflight_unix_test.go).

// newPreflightProject creates a project with a project file and one
// unrelated query, "other", written through the pair transaction (so the
// transaction directory and its lock exist, and every later read goes
// through recovery).
func newPreflightProject(t *testing.T) (fsProjectStore, string) {
	t.Helper()
	projectDir := t.TempDir()
	writePreflightProjectFile(t, projectDir)
	ps := newFsProjectStore("p", projectDir)
	other := dtqlQuery("other", "", "OTHER")
	if _, err := ps.PutQuery(context.Background(), &other, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error creating the unrelated query: %v", err)
	}
	return ps, filepath.Join(projectDir, storage.QueriesFolder)
}

func writePreflightProjectFile(t *testing.T, projectDir string) {
	t.Helper()
	data, err := json.Marshal(datatug.ProjectFile{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{Title: "Preflight"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, storage.ProjectSummaryFileName), data, 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// requireRefusedWrite fails the test unless err is a refusal naming want.
func requireRefusedWrite(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected the write to be refused (%q), got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected the refusal to mention %q, got: %v", want, err)
	}
}

// assertStoreUsableAfterRefusal checks that a refused write committed
// nothing and that every other query read and write of the project - and
// LoadProject's query tree - still works.
func assertStoreUsableAfterRefusal(t *testing.T, ps fsProjectStore, queriesDir string) {
	t.Helper()
	ctx := context.Background()
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	for _, name := range []string{queryTxnJournalFile, queryTxnJournalTmpFile, queryTxnStagedJSON, queryTxnStagedBody} {
		if _, err := os.Lstat(filepath.Join(txnDir, name)); !os.IsNotExist(err) {
			t.Errorf("expected no %s after a refused write, got Lstat err=%v", name, err)
		}
	}
	if q, err := ps.LoadQuery(ctx, "other"); err != nil {
		t.Errorf("LoadQuery(other) after a refused write: %v", err)
	} else if q.Text != "OTHER" {
		t.Errorf("LoadQuery(other): expected text %q, got %q", "OTHER", q.Text)
	}
	if _, err := ps.LoadQueryRevision(ctx, "other"); err != nil {
		t.Errorf("LoadQueryRevision(other) after a refused write: %v", err)
	}
	if _, err := ps.LoadQueries(ctx, ""); err != nil {
		t.Errorf("LoadQueries after a refused write: %v", err)
	}
	if project, err := ps.LoadProject(ctx); err != nil {
		t.Errorf("LoadProject after a refused write: %v", err)
	} else if project.Queries == nil || !queriesFolderHas(project.Queries, "other") {
		t.Errorf("LoadProject after a refused write: expected the query tree to hold %q, got %+v", "other", project.Queries)
	}
	third := dtqlQuery("third", "", "THIRD")
	if err := ps.SaveQuery(ctx, &third); err != nil {
		t.Errorf("SaveQuery(third) after a refused write: %v", err)
	}
}

func queriesFolderHas(folder *datatug.QueriesFolder, id string) bool {
	for _, item := range folder.Items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func TestPutQuery_RefusesADirectoryAtTheNewBodyNameAndStaysUsable(t *testing.T) {
	ps, queriesDir := newPreflightProject(t)
	if err := os.Mkdir(filepath.Join(queriesDir, "new.query.dtql"), 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	q := dtqlQuery("new", "", "NEW")
	_, err := ps.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	requireRefusedWrite(t, err, "not a regular file")
	if _, statErr := os.Lstat(filepath.Join(queriesDir, "new.query.json")); !os.IsNotExist(statErr) {
		t.Errorf("expected no metadata file installed for the refused write, got Lstat err=%v", statErr)
	}
	assertStoreUsableAfterRefusal(t, ps, queriesDir)

	// The legacy writers funnel through the same commit, so they refuse too.
	legacy := dtqlQuery("new", "", "NEW")
	requireRefusedWrite(t, ps.SaveQuery(context.Background(), &legacy), "not a regular file")
	assertStoreUsableAfterRefusal(t, ps, queriesDir)
}

func TestPutQuery_RefusesAnOverCapFileAtTheNewBodyNameAndStaysUsable(t *testing.T) {
	ps, queriesDir := newPreflightProject(t)
	// A sparse file over the read cap, e.g. an orphaned legacy sidecar.
	f, err := os.Create(filepath.Join(queriesDir, "new.query.dtql"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := f.Truncate(maxQueryFileSize + 1<<20); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = f.Close()
	q := dtqlQuery("new", "", "NEW")
	_, err = ps.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	requireRefusedWrite(t, err, "over the")
	assertStoreUsableAfterRefusal(t, ps, queriesDir)
}

func TestPutQuery_RefusesATypeChangeOntoADirectoryAndStaysUsable(t *testing.T) {
	ps, queriesDir := newPreflightProject(t)
	ctx := context.Background()
	tc := dtqlQuery("tc", "", "DTQL BODY")
	stored, err := ps.PutQuery(ctx, &tc, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}
	if err := os.Mkdir(filepath.Join(queriesDir, "tc.query.sql"), 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	changed := dtqlQuery("tc", "", "SELECT 1")
	changed.Type = datatug.QueryTypeSQL
	_, err = ps.PutQuery(ctx, &changed, datatug.QueryWriteCondition{IfMatch: stored.Revision})
	requireRefusedWrite(t, err, "tc.query.sql")
	assertStoreUsableAfterRefusal(t, ps, queriesDir)

	// The refused type change left the DTQL pair exactly as it was.
	got, err := ps.LoadQueryRevision(ctx, "tc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Revision != stored.Revision || got.Query.Type != datatug.QueryTypeDTQL || got.Query.Text != "DTQL BODY" {
		t.Errorf("expected the DTQL pair unchanged at revision %s, got type %s, text %q, revision %s", stored.Revision, got.Query.Type, got.Query.Text, got.Revision)
	}
}

// Once commitQueryTransaction refuses every transaction recovery would
// refuse, recovery can fail only if something outside DataTug changes a
// target after the commit. Its error then names the entry and says how to
// clear it, and the store recovers as soon as the entry is fixed.
func TestRecovery_ExplainsHowToClearATargetChangedAfterTheCommit(t *testing.T) {
	ps, queriesDir := newPreflightProject(t)
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	q := dtqlQuery("new", "", "NEW")
	jsonBytes, err := queryJSONBytes(q.QueryDef)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for name, data := range map[string][]byte{queryTxnStagedJSON: jsonBytes, queryTxnStagedBody: []byte(q.Text)} {
		if err := writeStagedFile(txnDir, name, data); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	j := queryTxnJournal{
		ID: "new", Operation: queryTxnOpPut,
		JSONFileName: "new.query.json", JSONHash: hashBytes(jsonBytes),
		BodyFileName: "new.query.dtql", BodyHash: hashBytes([]byte(q.Text)),
	}
	if err := commitQueryTransaction(queriesDir, txnDir, j); err != nil {
		t.Fatalf("unexpected error committing: %v", err)
	}
	// Tampering after the commit point.
	target := filepath.Join(queriesDir, "new.query.dtql")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = ps.LoadQueries(context.Background(), "")
	for _, want := range []string{"cannot be completed", "new.query.dtql", "next access", "Do not remove " + filepath.ToSlash(filepath.Join(txnDir, queryTxnJournalFile))} {
		if err == nil || !strings.Contains(filepath.ToSlash(err.Error()), want) {
			t.Errorf("expected the recovery error to mention %q, got: %v", want, err)
		}
	}
	if err != nil && strings.Contains(err.Error(), "abandon") {
		t.Errorf("expected the advice never to offer abandoning the transaction (review SF-B), got: %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := ps.LoadQuery(context.Background(), "new")
	if err != nil {
		t.Fatalf("expected recovery to finish once the entry is removed, got: %v", err)
	}
	if got.Text != "NEW" {
		t.Errorf("expected the committed content %q installed, got %q", "NEW", got.Text)
	}
}

func TestCommitQueryTransaction_RefusesAnInvalidJournalWithoutCommitting(t *testing.T) {
	_, queriesDir := newPreflightProject(t)
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	err := commitQueryTransaction(queriesDir, txnDir, queryTxnJournal{ID: "x", Operation: "bogus", JSONFileName: "x.query.json"})
	requireRefusedWrite(t, err, "invalid query transaction journal")
	if _, statErr := os.Lstat(filepath.Join(txnDir, queryTxnJournalFile)); !os.IsNotExist(statErr) {
		t.Errorf("expected no journal committed, got Lstat err=%v", statErr)
	}
}
