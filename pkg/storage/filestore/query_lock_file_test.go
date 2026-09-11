package filestore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Review S3 regression tests (lockplant): the lock file must be the user's
// own regular file, and opening it must never follow a symlink or block
// on a FIFO - whether the transaction directory is private or arrives with
// broad permissions and is repaired.

// newTxnDirForLockTest creates an empty transaction directory, private
// (0700) or broad (0777).
func newTxnDirForLockTest(t *testing.T, broad bool) (projectDir, txnDir string) {
	t.Helper()
	projectDir = t.TempDir()
	txnDir = filepath.Join(projectDir, "queries", reservedQueryTxnDirName)
	if err := os.MkdirAll(txnDir, 0o700); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mode := os.FileMode(0o700)
	if broad {
		mode = 0o777
	}
	if err := os.Chmod(txnDir, mode); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return projectDir, txnDir
}

func TestQueryLock_RefusesAPlantedLockSymlink(t *testing.T) {
	skipSymlinksOnWindows(t)
	for _, broad := range []bool{false, true} {
		t.Run(fmt.Sprintf("broad=%v", broad), func(t *testing.T) {
			projectDir, txnDir := newTxnDirForLockTest(t, broad)
			target := filepath.Join(t.TempDir(), "created-via-lock-symlink")
			lock := filepath.Join(txnDir, queryTxnLockFile)
			if err := os.Symlink(target, lock); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			store := newFsQueriesStore(projectDir)
			ctx := context.Background()
			if _, err := store.LoadQueries(ctx, ""); err == nil || !strings.Contains(err.Error(), "symlink") {
				t.Errorf("LoadQueries: expected a symlinked lock to be refused, got: %v", err)
			}
			q := dtqlQuery("q", "", "B")
			if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err == nil {
				t.Error("PutQuery: expected a symlinked lock to be refused")
			}
			if fileExistsAt(target) {
				t.Error("expected nothing created at the symlinked lock's target")
			}
			if info, err := os.Lstat(lock); err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Errorf("expected the planted symlink left in place, got %v (err %v)", info, err)
			}
		})
	}
}

func TestQueryLock_RefusesADirectoryAsTheLock(t *testing.T) {
	projectDir, txnDir := newTxnDirForLockTest(t, false)
	if err := os.Mkdir(filepath.Join(txnDir, queryTxnLockFile), 0o700); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := newFsQueriesStore(projectDir).LoadQueries(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected a directory lock to be refused, got: %v", err)
	}
}

func TestQueryLock_RefusesALockOwnedByAnotherUser(t *testing.T) {
	for _, broad := range []bool{false, true} {
		t.Run(fmt.Sprintf("broad=%v", broad), func(t *testing.T) {
			if broad && runtime.GOOS == "windows" {
				t.Skip("Go's os.FileMode permission bits are synthetic on Windows")
			}
			projectDir, txnDir := newTxnDirForLockTest(t, broad)
			writeFile0600(t, filepath.Join(txnDir, queryTxnLockFile), nil)
			withFileOwnership(t, func(info os.FileInfo) bool { return info.Name() != queryTxnLockFile })
			if _, err := newFsQueriesStore(projectDir).LoadQueries(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "owned by another user") {
				t.Fatalf("expected a foreign lock to be refused, got: %v", err)
			}
		})
	}
}

// TestQueryLock_AcceptsItsOwnRegularLockAfterAnArchiveRoundTrip proves the
// check is about the lock's type and owner, not its permission bits: an
// archive round trip that leaves the user's own lock at 0644 (and the
// directory at 0755) must keep working.
func TestQueryLock_AcceptsItsOwnRegularLockAfterAnArchiveRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Go's os.FileMode permission bits are synthetic on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission checks")
	}
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q", "", "B0")
	created, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	if err := os.Chmod(filepath.Join(txnDir, queryTxnLockFile), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.Chmod(txnDir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reopened := newFsQueriesStore(filepath.Dir(queriesDir))
	if _, err := reopened.LoadQueryRevision(ctx, "q"); err != nil {
		t.Fatalf("LoadQueryRevision: %v", err)
	}
	updated := dtqlQuery("q", "", "B1")
	if _, err := reopened.PutQuery(ctx, &updated, datatug.QueryWriteCondition{IfMatch: created.Revision}); err != nil {
		t.Fatalf("PutQuery: %v", err)
	}
}
