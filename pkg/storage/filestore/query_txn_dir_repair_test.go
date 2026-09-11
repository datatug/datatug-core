package filestore

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// TestQueryStore_SurvivesAZipUnzipRoundTrip is B1's end-to-end regression
// test: a project that has already had one query written through this
// store (so ".dt-query-txn" exists, holding only its own ".gitignore" and
// - once a lock has ever been taken - a "lock" file, never a journal or
// staged content once a transaction has completed) gets zipped and
// unzipped, exactly the way Python's stdlib zipfile.extractall() does it -
// which does not preserve the 0700 permission bit and leaves the
// directory at 0755. Every subsequent read and write against that project
// must keep working, not be permanently bricked by a hidden dot-directory
// an ordinary user has no reason to know needs a chmod.
func TestQueryStore_SurvivesAZipUnzipRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Go's os.FileMode permission bits are synthetic on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission checks")
	}
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "from:\n  name: Invoice\n")
	stored, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	if err := os.Chmod(txnDir, 0o755); err != nil {
		t.Fatalf("unexpected error simulating an extraction round trip: %v", err)
	}

	// A fresh store instance, as a new process opening the unzipped
	// project would be.
	reopened := newFsQueriesStore(filepath.Dir(queriesDir))

	if _, err := reopened.LoadQuery(ctx, "q1"); err != nil {
		t.Fatalf("LoadQuery after the round trip: %v", err)
	}
	if _, err := reopened.LoadQueryRevision(ctx, "q1"); err != nil {
		t.Fatalf("LoadQueryRevision after the round trip: %v", err)
	}
	updated := dtqlQuery("q1", "", "updated")
	if _, err := reopened.PutQuery(ctx, &updated, datatug.QueryWriteCondition{IfMatch: stored.Revision}); err != nil {
		t.Fatalf("PutQuery after the round trip: %v", err)
	}

	info, err := os.Lstat(txnDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("expected the transaction directory to be repaired to 0700, got %v", perm)
	}
}
