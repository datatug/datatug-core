package filestore

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// TestReads_DoNotRequireWriteAccessOnAnUntouchedProject is a regression
// test: an earlier version of withQueryLock acquired the query store lock
// - which lazily creates the reserved ".dt-query-txn" directory - for
// every operation, reads included. That meant simply opening/reading a
// project whose "queries/" directory lives on a read-only mount, or one
// this store had simply never written to before, failed (or, worse,
// silently wrote a stray directory into it - this is exactly what
// happened running this branch's datatug-cli compatibility check against
// a real read-only-intended fixture checkout). withQueryReadLock fixes
// this: a project no write has ever touched has nothing to coordinate
// against, so a read never needs write access.
func TestReads_DoNotRequireWriteAccessOnAnUntouchedProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission checks")
	}

	projectDir := t.TempDir()
	queriesDir := filepath.Join(projectDir, "queries")
	if err := os.MkdirAll(filepath.Join(queriesDir, "customers"), 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(queriesDir, "customers", "c1.query.json"),
		[]byte(`{"id":"c1","title":"C1","type":"SQL"}`+"\n"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(queriesDir, "customers", "c1.query.sql"),
		[]byte("SELECT 1"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Make every directory in the tree read-only, as a read-only mount or
	// a checkout the caller must not write to would be.
	for _, dir := range []string{projectDir, queriesDir, filepath.Join(queriesDir, "customers")} {
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	t.Cleanup(func() {
		for _, dir := range []string{filepath.Join(queriesDir, "customers"), queriesDir, projectDir} {
			_ = os.Chmod(dir, 0o755)
		}
	})

	store := newFsQueriesStore(projectDir)
	ctx := context.Background()

	if _, err := store.LoadQueries(ctx, "customers"); err != nil {
		t.Errorf("LoadQueries on a read-only project: %v", err)
	}
	if _, err := store.LoadQuery(ctx, "customers/c1"); err != nil {
		t.Errorf("LoadQuery on a read-only project: %v", err)
	}
	if _, err := store.loadQueriesTree(ctx, ""); err != nil {
		t.Errorf("loadQueriesTree on a read-only project: %v", err)
	}
	if _, err := store.LoadQueryRevision(ctx, "customers/c1"); err != nil {
		t.Errorf("LoadQueryRevision on a read-only project: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(queriesDir, reservedQueryTxnDirName)); !os.IsNotExist(err) {
		t.Errorf("expected no reserved transaction namespace to be created by reads alone, Lstat err: %v", err)
	}
}

// TestWithQueryReadLock_StillCoordinatesOnceProjectHasBeenWrittenTo proves
// the trade-off is not silently unsafe: once a write has happened (so the
// reserved namespace exists), a subsequent read still goes through the
// real lock and recovery path, not the fast "nothing to coordinate" one.
func TestWithQueryReadLock_StillCoordinatesOnceProjectHasBeenWrittenTo(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "text")
	if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(queriesDir, reservedQueryTxnDirName)); err != nil {
		t.Fatalf("expected the reserved namespace to exist after a write: %v", err)
	}

	// Seed an interrupted transaction the way the earlier recovery tests
	// do, then prove a plain read recovers it - which only happens when
	// the read goes through withQueryLock, not the read-only fast path.
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jsonBytes := []byte(`{"id":"q2","title":"Q2","type":"DTQL"}` + "\n")
	bodyBytes := []byte("recovered")
	if err := writeStagedFile(txnDir, queryTxnStagedJSON, jsonBytes); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := writeStagedFile(txnDir, queryTxnStagedBody, bodyBytes); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	writeTestJournal(t, txnDir, queryTxnJournal{
		FolderPath: "", ID: "q2", Operation: queryTxnOpPut,
		JSONFileName: "q2.query.json", JSONHash: hashBytes(jsonBytes),
		BodyFileName: "q2.query.dtql", BodyHash: hashBytes(bodyBytes),
	})

	loaded, err := store.LoadQuery(ctx, "q2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded.Text != "recovered" {
		t.Fatalf("expected LoadQuery to recover the interrupted transaction, got %q", loaded.Text)
	}
}
