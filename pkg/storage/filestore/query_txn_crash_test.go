package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Review B2 regression tests: a crash before the commit point must never
// leave the store unable to read or write, the commit point must be
// atomic, and a failed journal write must not strand its staged files.

func TestRecovery_SweepsUncommittedLeftovers(t *testing.T) {
	for _, tc := range []struct {
		name      string
		leftovers map[string]string
	}{
		{"staged json only (leftover mode)", map[string]string{queryTxnStagedJSON: "partial"}},
		{"staged body only", map[string]string{queryTxnStagedBody: "partial"}},
		{"both staged files", map[string]string{queryTxnStagedJSON: "j", queryTxnStagedBody: "b"}},
		{"empty journal.tmp: killed between its create and write", map[string]string{queryTxnJournalTmpFile: ""}},
		{"partial journal.tmp with both staged files", map[string]string{
			queryTxnJournalTmpFile: `{"id":"q","oper`, queryTxnStagedJSON: "j", queryTxnStagedBody: "b",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, queriesDir := newTestQueriesStore(t)
			ctx := context.Background()
			q := dtqlQuery("q", "", "B0")
			created, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
			if err != nil {
				t.Fatalf("unexpected error creating: %v", err)
			}
			txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
			for name, data := range tc.leftovers {
				writeFile0600(t, filepath.Join(txnDir, name), []byte(data))
			}

			fresh := newFsQueriesStore(filepath.Dir(queriesDir))
			cur, err := fresh.LoadQueryRevision(ctx, "q")
			if err != nil {
				t.Fatalf("LoadQueryRevision after the crash: %v", err)
			}
			if cur.Query.Text != "B0" || cur.Revision != created.Revision {
				t.Fatalf("expected the previous complete revision untouched, got text %q revision %s", cur.Query.Text, cur.Revision)
			}
			for name := range tc.leftovers {
				if fileExistsAt(filepath.Join(txnDir, name)) {
					t.Errorf("expected the uncommitted leftover %s to be swept", name)
				}
			}
			updated := dtqlQuery("q", "", "B1")
			if _, err := fresh.PutQuery(ctx, &updated, datatug.QueryWriteCondition{IfMatch: cur.Revision}); err != nil {
				t.Errorf("PutQuery after the crash: %v", err)
			}
			other := dtqlQuery("other", "", "O")
			if err := fresh.SaveQuery(ctx, &other); err != nil {
				t.Errorf("SaveQuery after the crash: %v", err)
			}
			if err := fresh.DeleteQuery(ctx, "other"); err != nil {
				t.Errorf("DeleteQuery after the crash: %v", err)
			}
			if _, err := fresh.LoadQueries(ctx, ""); err != nil {
				t.Errorf("LoadQueries after the crash: %v", err)
			}
		})
	}
}

// TestRecovery_EmptyCommittedJournalStillFailsClosed documents the other
// half of the emptyjournal repro: writeJournal can no longer produce an
// empty journal.json (a crash now leaves journal.tmp, swept above), and a
// journal.json that is empty anyway - tampering, or a file system that
// lost the data of a completed rename - holds no committed intent to
// finish, so it is refused like any corrupt journal and nothing is
// touched.
func TestRecovery_EmptyCommittedJournalStillFailsClosed(t *testing.T) {
	p := plantTxn(t, queryTxnJournal{ID: "q", Operation: queryTxnOpDelete, JSONFileName: "q.query.json"}, []byte("j"), nil)
	writeFile0600(t, filepath.Join(p.txnDir, queryTxnJournalFile), nil)
	before := snapshotTxnArtifacts(t, p.txnDir)
	if _, err := newFsQueriesStore(p.projectDir).LoadQueries(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("expected an empty committed journal to be refused as corrupt, got: %v", err)
	}
	assertSameSnapshot(t, "transaction directory", before, snapshotTxnArtifacts(t, p.txnDir))
}

func TestRecovery_RefusesAnUntrustworthyLeftoverAndTouchesNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows CI")
	}
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q", "", "B0")
	if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("KEEP"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	writeFile0600(t, filepath.Join(txnDir, queryTxnStagedBody), []byte("b"))
	if err := os.Symlink(outside, filepath.Join(txnDir, queryTxnStagedJSON)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	before := snapshotTxnArtifacts(t, txnDir)

	if _, err := newFsQueriesStore(filepath.Dir(queriesDir)).LoadQueries(ctx, ""); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected a symlinked leftover to be refused, got: %v", err)
	}
	assertSameSnapshot(t, "transaction directory", before, snapshotTxnArtifacts(t, txnDir))
	if b, err := os.ReadFile(outside); err != nil || string(b) != "KEEP" {
		t.Errorf("expected the symlink target untouched, got %q (err %v)", b, err)
	}

	t.Run("owned by another user", func(t *testing.T) {
		if err := os.Remove(filepath.Join(txnDir, queryTxnStagedJSON)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		writeFile0600(t, filepath.Join(txnDir, queryTxnJournalTmpFile), []byte("{"))
		withFileOwnership(t, func(info os.FileInfo) bool { return info.Name() != queryTxnJournalTmpFile })
		before := snapshotTxnArtifacts(t, txnDir)
		if _, err := newFsQueriesStore(filepath.Dir(queriesDir)).LoadQueries(ctx, ""); err == nil || !strings.Contains(err.Error(), "owned by another user") {
			t.Fatalf("expected a foreign leftover to be refused, got: %v", err)
		}
		assertSameSnapshot(t, "transaction directory", before, snapshotTxnArtifacts(t, txnDir))
	})
}

func TestWriteJournal_CommitsThroughAnAtomicRename(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	j := queryTxnJournal{FolderPath: "a/b", ID: "q", Operation: queryTxnOpDelete, JSONFileName: "q.query.json", BodyFileName: "q.query.sql"}
	if err := writeJournal(txnDir, j); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(txnDir, queryTxnJournalFile))
	if err != nil {
		t.Fatalf("expected journal.json to exist: %v", err)
	}
	var got queryTxnJournal
	if err := json.Unmarshal(b, &got); err != nil || got != j {
		t.Fatalf("expected the committed journal to round-trip, got %+v (err %v)", got, err)
	}
	if fileExistsAt(filepath.Join(txnDir, queryTxnJournalTmpFile)) {
		t.Error("expected journal.tmp to be renamed away")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Lstat(filepath.Join(txnDir, queryTxnJournalFile))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if perm := info.Mode().Perm(); perm != queryTxnFilePermMode {
			t.Errorf("expected the journal at %v, got %v", os.FileMode(queryTxnFilePermMode), perm)
		}
	}
}

func TestWriteJournal_RefusesAJournalRecoveryWouldRefuse(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = writeJournal(txnDir, queryTxnJournal{ID: "q", Operation: queryTxnOpDelete, JSONFileName: "q.query.json", BodyFileName: "../../x"})
	if err == nil {
		t.Fatal("expected an invalid journal to be refused before it is committed")
	}
	if fileExistsAt(filepath.Join(txnDir, queryTxnJournalFile)) || fileExistsAt(filepath.Join(txnDir, queryTxnJournalTmpFile)) {
		t.Error("expected nothing written for a refused journal")
	}
}

func TestWriteJournal_NeverRemovesAJournalTmpItDidNotCreate(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	writeFile0600(t, filepath.Join(txnDir, queryTxnJournalTmpFile), []byte("not mine"))
	if err := writeJournal(txnDir, queryTxnJournal{ID: "q", Operation: queryTxnOpDelete, JSONFileName: "q.query.json"}); err == nil {
		t.Fatal("expected exclusive creation of journal.tmp to fail")
	}
	if b, _ := os.ReadFile(filepath.Join(txnDir, queryTxnJournalTmpFile)); string(b) != "not mine" {
		t.Errorf("expected the existing journal.tmp untouched, got %q", b)
	}
	if fileExistsAt(filepath.Join(txnDir, queryTxnJournalFile)) {
		t.Error("expected no journal committed")
	}
}

func TestStageAndInstall_DiscardsStagedFilesWhenTheJournalCannotBeWritten(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A committed journal already present makes writeJournal fail after
	// both files are staged (it never replaces a committed journal).
	writeFile0600(t, filepath.Join(txnDir, queryTxnJournalFile), []byte("{}"))
	q := dtqlQuery("q", "", "B")
	if _, err := store.stageAndInstallQueryPair(queryLockGuard{txnDir: txnDir}, queriesDir, "", q.QueryDef, currentQueryPair{}); err == nil {
		t.Fatal("expected the journal write to fail")
	}
	for _, name := range []string{queryTxnStagedJSON, queryTxnStagedBody, queryTxnJournalTmpFile} {
		if fileExistsAt(filepath.Join(txnDir, name)) {
			t.Errorf("expected %s to be discarded after the journal write failed", name)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(txnDir, queryTxnJournalFile)); string(b) != "{}" {
		t.Errorf("expected the existing journal untouched, got %q", b)
	}
	if fileExistsAt(filepath.Join(queriesDir, "q.query.json")) {
		t.Error("expected nothing installed")
	}
}

func TestQueryWrites_RefuseAFileOverTheSizeCap(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	big := dtqlQuery("big", "", strings.Repeat("x", maxQueryFileSize+1))

	if _, err := store.PutQuery(ctx, &big, datatug.QueryWriteCondition{IfNoneMatch: true}); err == nil {
		t.Fatal("PutQuery: expected a body over the cap to be refused")
	}
	if fileExistsAt(filepath.Join(queriesDir, reservedQueryTxnDirName)) {
		t.Error("PutQuery: expected the refusal before the lock, creating nothing")
	}
	if err := store.SaveQuery(ctx, &big); err == nil {
		t.Fatal("SaveQuery: expected a body over the cap to be refused")
	}
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	for _, name := range []string{queryTxnStagedJSON, queryTxnStagedBody, queryTxnJournalFile, queryTxnJournalTmpFile} {
		if fileExistsAt(filepath.Join(txnDir, name)) {
			t.Errorf("SaveQuery: expected no %s after the refusal", name)
		}
	}
	if fileExistsAt(filepath.Join(queriesDir, "big.query.json")) {
		t.Error("SaveQuery: expected nothing installed")
	}
}

func TestEnsureQueryTxnDir_RefusesBroadPermissionsWithJournalTmpPresent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Go's os.FileMode permission bits are synthetic on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission checks")
	}
	root := t.TempDir()
	queriesDir := filepath.Join(root, "queries")
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	if err := os.MkdirAll(txnDir, 0o777); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.Chmod(txnDir, 0o777); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	writeFile0600(t, filepath.Join(txnDir, queryTxnJournalTmpFile), []byte("{}"))
	if _, err := ensureQueryTxnDir(queriesDir); err == nil {
		t.Fatal("expected a world-writable transaction directory holding journal.tmp to be refused")
	}
}
