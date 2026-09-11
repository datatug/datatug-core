package filestore

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Task 5: deterministic filesystem fault injection at every transaction
// phase, proving the store fails closed rather than trusting a corrupt or
// tampered recovery artifact.

func TestEnsureQueryTxnDir_RejectsNonDirectory(t *testing.T) {
	root := t.TempDir()
	queriesDir := filepath.Join(root, "queries")
	if err := os.MkdirAll(queriesDir, 0o777); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(queriesDir, reservedQueryTxnDirName), []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := ensureQueryTxnDir(queriesDir); err == nil {
		t.Fatal("expected an error when the reserved namespace exists as a regular file")
	}
}

func TestEnsureQueryTxnDir_RejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows CI")
	}
	root := t.TempDir()
	queriesDir := filepath.Join(root, "queries")
	if err := os.MkdirAll(queriesDir, 0o777); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(queriesDir, reservedQueryTxnDirName)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := ensureQueryTxnDir(queriesDir); err == nil {
		t.Fatal("expected an error when the reserved namespace is a symlink")
	}
}

// TestEnsureQueryTxnDir_RepairsEmptyDirWithBroadPermissions is a
// regression test for B1: an empty ".dt-query-txn" (no journal, no staged
// content - nothing a looser permission bit could let anyone tamper with)
// that comes back from an ordinary zip/unzip, tar, Dropbox or AV-quarantine
// round trip at 0755 used to be refused outright, permanently bricking
// every read and write against the project. There is nothing to trust or
// distrust in an empty directory, so ensureQueryTxnDir now repairs it back
// to 0700 in place and proceeds, exactly like the "case == 0700" path.
func TestEnsureQueryTxnDir_RepairsEmptyDirWithBroadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Go's os.FileMode permission bits are synthetic on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission checks")
	}
	root := t.TempDir()
	queriesDir := filepath.Join(root, "queries")
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	// 0755 is exactly what Python's zipfile.extractall() (and many other
	// common archivers) leaves behind - it does not preserve the 0700 bit.
	if err := os.MkdirAll(txnDir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("expected an empty over-permissioned directory to be repaired, not refused: %v", err)
	}
	if got != txnDir {
		t.Fatalf("expected %s, got %s", txnDir, got)
	}
	info, err := os.Lstat(txnDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("expected the directory to be repaired to 0700, got %v", perm)
	}
}

// TestEnsureQueryTxnDir_RejectsBroadPermissionsWithJournalPresent proves
// the repair in TestEnsureQueryTxnDir_RepairsEmptyDirWithBroadPermissions
// is narrowly scoped: a transaction directory that actually holds a
// journal (a real recovery artifact something could have tampered with)
// still fails closed when its permissions are broader than 0700, exactly
// as before - and is left untouched, not silently repaired.
func TestEnsureQueryTxnDir_RejectsBroadPermissionsWithJournalPresent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Go's os.FileMode permission bits are synthetic on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission checks")
	}
	root := t.TempDir()
	queriesDir := filepath.Join(root, "queries")
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	if err := os.MkdirAll(txnDir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(txnDir, queryTxnJournalFile), []byte(`{"id":"q1","operation":"put"}`), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := ensureQueryTxnDir(queriesDir); err == nil {
		t.Fatal("expected an error for a world-readable transaction directory holding a journal")
	}
	info, err := os.Lstat(txnDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o755 {
		t.Fatalf("expected the directory's permissions to be left untouched, got %v", perm)
	}
}

// TestEnsureQueryTxnDir_RejectsBroadPermissionsWithStagedContentPresent
// covers the other recovery artifacts the repair must not silently trust:
// a staged (not yet journaled) file.
func TestEnsureQueryTxnDir_RejectsBroadPermissionsWithStagedContentPresent(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(txnDir, queryTxnStagedJSON), []byte(`{"id":"q1"}`), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := ensureQueryTxnDir(queriesDir); err == nil {
		t.Fatal("expected an error for a world-writable transaction directory holding staged content")
	}
}

func TestEnsureQueryTxnDir_AcceptsExisting0700Directory(t *testing.T) {
	root := t.TempDir()
	queriesDir := filepath.Join(root, "queries")
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	if err := os.MkdirAll(txnDir, 0o700); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != txnDir {
		t.Fatalf("expected %s, got %s", txnDir, got)
	}
}

func TestReadJournal_RejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows CI")
	}
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elsewhere := filepath.Join(t.TempDir(), "elsewhere.json")
	if err := os.WriteFile(elsewhere, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(txnDir, queryTxnJournalFile)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, _, err := readJournal(txnDir); err == nil {
		t.Fatal("expected an error when the journal is a symlink")
	}
}

func TestReadJournal_RejectsOverlyBroadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Go's os.FileMode permission bits are synthetic on Windows")
	}
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(txnDir, queryTxnJournalFile), []byte(`{"id":"q1","operation":"put"}`), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, _, err := readJournal(txnDir); err == nil {
		t.Fatal("expected an error for a world-readable journal file")
	}
}

func TestReadJournal_RejectsCorruptJSON(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(txnDir, queryTxnJournalFile), []byte("not json"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, _, err := readJournal(txnDir); err == nil {
		t.Fatal("expected an error for corrupt journal JSON")
	}
}

func TestReadJournal_RejectsUnrecognizedOperation(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	writeTestJournal(t, txnDir, queryTxnJournal{FolderPath: "", ID: "q1", Operation: "rename", JSONFileName: "q1.query.json"})
	if _, _, err := readJournal(txnDir); err == nil {
		t.Fatal("expected an error for an unrecognized operation")
	}
}

func TestReadJournal_RejectsInvalidFolderPath(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A journal is only ever written by this store's own validated writes,
	// but recovery must not simply trust it - simulate tampering/corruption.
	writeTestJournal(t, txnDir, queryTxnJournal{FolderPath: "../escape", ID: "q1", Operation: queryTxnOpPut, JSONFileName: "q1.query.json"})
	if _, _, err := readJournal(txnDir); err == nil {
		t.Fatal("expected an error for an invalid folder path in the journal")
	}
}

func TestReadJournal_RejectsInvalidID(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	writeTestJournal(t, txnDir, queryTxnJournal{FolderPath: "", ID: "../escape", Operation: queryTxnOpPut, JSONFileName: "x.query.json"})
	if _, _, err := readJournal(txnDir); err == nil {
		t.Fatal("expected an error for an invalid id in the journal")
	}
}

func TestEnsureInstalled_RejectsStagedHashMismatch(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.MkdirAll(queriesDir, 0o777); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := writeStagedFile(txnDir, queryTxnStagedJSON, []byte("tampered content")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = ensureInstalled(txnDir, queryTxnStagedJSON, queriesDir, "q1.query.json", hashBytes([]byte("expected content")))
	if err == nil {
		t.Fatal("expected an error when the staged content doesn't match its recorded hash")
	}
	if fileExistsAt(filepath.Join(queriesDir, "q1.query.json")) {
		t.Error("expected nothing to be installed when the staged hash doesn't match")
	}
}

func TestEnsureInstalled_RejectsMissingStagedFileWhenFinalDoesNotMatch(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.MkdirAll(queriesDir, 0o777); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = ensureInstalled(txnDir, queryTxnStagedJSON, queriesDir, "q1.query.json", hashBytes([]byte("expected content")))
	if err == nil {
		t.Fatal("expected an error when neither the staged file nor a matching final file exists")
	}
}

func TestEnsureInstalled_IsANoOpWhenAlreadyInstalled(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.MkdirAll(queriesDir, 0o777); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content := []byte("already installed")
	if err := os.WriteFile(filepath.Join(queriesDir, "q1.query.json"), content, 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No staged file at all - ensureInstalled must recognize the final file
	// already matches and do nothing, rather than erroring on the missing
	// staged file.
	if err := ensureInstalled(txnDir, queryTxnStagedJSON, queriesDir, "q1.query.json", hashBytes(content)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteStagedFile_RejectsExistingFile(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(txnDir, queryTxnStagedJSON), []byte("stale leftover"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := writeStagedFile(txnDir, queryTxnStagedJSON, []byte("new content")); err == nil {
		t.Fatal("expected exclusive creation to reject an already-present staged file")
	}
}

func TestWriteJournal_RejectsExistingJournal(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(txnDir, queryTxnJournalFile), []byte("stale leftover"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = writeJournal(txnDir, queryTxnJournal{FolderPath: "", ID: "q1", Operation: queryTxnOpPut, JSONFileName: "q1.query.json"})
	if err == nil {
		t.Fatal("expected exclusive creation to reject an already-present journal")
	}
}

func TestRemoveIfExists_PropagatesNonNotExistError(t *testing.T) {
	root := t.TempDir()
	nonEmptyDir := filepath.Join(root, "not-a-plain-file")
	if err := os.MkdirAll(filepath.Join(nonEmptyDir, "child"), 0o777); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// os.Remove refuses a non-empty directory - a real error distinct from
	// "already gone", which removeIfExists must propagate rather than
	// swallow.
	if err := removeIfExists(nonEmptyDir); err == nil {
		t.Fatal("expected removeIfExists to propagate a real removal error")
	}
}

func TestCompleteQueryTransaction_RejectsCorruptJournalWithoutTouchingFiles(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "original")
	if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(queriesDir, "q1.query.dtql"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(txnDir, queryTxnJournalFile), []byte("garbage"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := completeQueryTransaction(queriesDir, txnDir); err == nil {
		t.Fatal("expected an error for a corrupt journal")
	}
	after, err := os.ReadFile(filepath.Join(queriesDir, "q1.query.dtql"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("expected the existing pair to be untouched, got %q", after)
	}
}
