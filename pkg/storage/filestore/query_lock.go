package filestore

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime"
	"time"

	"github.com/gofrs/flock"
)

// queryLockRetryDelay is how often TryLockContext re-attempts the
// cross-process advisory lock while it waits.
const queryLockRetryDelay = 20 * time.Millisecond

// queryLockGuard is proof the caller already holds the query store's
// single advisory lock, and that any earlier interrupted transaction has
// already been recovered. Only withQueryLock constructs one. A recursive
// internal helper (the query-tree loader/saver, a per-pair transaction
// step) accepts a queryLockGuard as a parameter instead of calling
// withQueryLock again: the lock is not reentrant - a second acquisition
// attempt from the same process would deadlock against the first, since
// gofrs/flock's platform locks are associated with the open file
// description, not the process.
type queryLockGuard struct {
	// txnDir is the reserved ".dt-query-txn" directory this lock, its
	// journal and its staging files live under.
	txnDir string
}

// withQueryLock acquires the query store's cross-process advisory lock,
// recovers any earlier interrupted transaction left by a previous holder
// (so "a fresh store instance recovers any interrupted journal" holds for
// every entry point, not just writes), then calls fn while still holding
// the lock. It is the sole place a query-store operation may acquire this
// lock.
//
// Waiting for the lock is cancellation-aware: if ctx is already done, or
// becomes done before the lock is acquired, withQueryLock returns its
// error without creating or touching anything but the reserved
// transaction directory itself (created once, lazily, the first time any
// query-store operation runs against a project). Once fn is called, ctx is
// no longer consulted by withQueryLock itself - fn's own operations must
// check ctx at their own reversible boundaries and, once a transaction's
// commit point is reached, must complete regardless of it (see
// completeQueryTransaction, which takes no context at all: recovery and
// commit-completion are never abandoned partway).
func (s fsQueriesStore) withQueryLock(ctx context.Context, fn func(g queryLockGuard) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	txnDir, err := ensureQueryTxnDir(s.dirPath)
	if err != nil {
		return err
	}
	lockPath := path.Join(txnDir, "lock")
	fl := flock.New(lockPath, flock.SetPermissions(0o600))
	ok, err := fl.TryLockContext(ctx, queryLockRetryDelay)
	if err != nil {
		return fmt.Errorf("failed to acquire the query store lock: %w", err)
	}
	if !ok {
		return fmt.Errorf("failed to acquire the query store lock")
	}
	defer func() { _ = fl.Unlock() }()

	if err := completeQueryTransaction(s.dirPath, txnDir); err != nil {
		return fmt.Errorf("failed to recover an earlier interrupted query transaction: %w", err)
	}

	return fn(queryLockGuard{txnDir: txnDir})
}

// withQueryReadLock is withQueryLock for a caller that only reads (fn must
// not stage, install or delete anything). If the reserved transaction
// namespace does not exist yet, nothing has ever been written through this
// store's pair-transaction protocol against this project, so there is
// nothing to coordinate against or recover: fn runs directly, without
// creating any file or directory and without requiring write access to
// the project - a project opened read-only, or simply never yet touched
// by a revisioned/legacy write, stays fully readable. queryLockGuard{} is
// the zero value in that path; no read-only caller dereferences its
// txnDir.
//
// Once that namespace exists - any write, ever, through
// PutQuery/SaveQuery/CreateQuery/UpdateQuery/DeleteQuery/saveQueriesTree -
// reads go through the same lock and recovery a write would, via
// withQueryLock. That still works on a project whose filesystem has since
// become read-only: the lock file the earlier write already created is
// merely opened and flock()'d, which needs no write permission on an
// existing file.
func (s fsQueriesStore) withQueryReadLock(ctx context.Context, fn func(g queryLockGuard) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	txnDir := path.Join(s.dirPath, reservedQueryTxnDirName)
	if info, err := os.Lstat(txnDir); err != nil || !info.IsDir() {
		return fn(queryLockGuard{})
	}
	return s.withQueryLock(ctx, fn)
}

// ensureQueryTxnDir returns the reserved ".dt-query-txn" directory under
// queriesRoot, creating it (and queriesRoot itself) at 0700 if it does not
// exist yet. If it already exists, it must be an ordinary directory
// (a symlink or a regular file is refused outright); when its permissions
// are broader than 0700, what happens next depends on what it holds. A
// journal or staged file is a real recovery artifact - something a
// tampering actor could have loosened permissions on to smuggle in
// content this store would otherwise trust - so that case still fails
// closed, per the plan's "fail closed if existing recovery artifacts are
// broader" rule. An otherwise-empty directory holds nothing to trust or
// distrust: the common real-world way to end up here is an ordinary
// zip/unzip, tar, Dropbox sync or AV-quarantine round trip, none of which
// preserve the 0700 bit (Python's zipfile.extractall(), for one, leaves
// 0755) - so that case is repaired back to 0700 in place and used, rather
// than permanently bricking every read and write against the project over
// a hidden dot-directory an ordinary user has no reason to know needs a
// chmod. Windows does not expose POSIX permission bits through
// os.FileMode (Go reports a synthetic value there), so the permission
// check only applies on the platforms where it is meaningful.
func ensureQueryTxnDir(queriesRoot string) (string, error) {
	txnDir := path.Join(queriesRoot, reservedQueryTxnDirName)
	info, err := os.Lstat(txnDir)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("query transaction directory %s is a symlink; refusing to use it", txnDir)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("query transaction path %s is not a directory; refusing to use it", txnDir)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&^0o700 != 0 {
			hasContent, contentErr := queryTxnDirHasRecoveryContent(txnDir)
			if contentErr != nil {
				return "", contentErr
			}
			if hasContent {
				return "", fmt.Errorf("query transaction directory %s has overly broad permissions %v; refusing to use it", txnDir, info.Mode().Perm())
			}
			if err := os.Chmod(txnDir, 0o700); err != nil {
				return "", fmt.Errorf("failed to repair query transaction directory permissions: %w", err)
			}
		}
		return txnDir, nil
	case os.IsNotExist(err):
		if err := os.MkdirAll(queriesRoot, 0o777); err != nil {
			return "", fmt.Errorf("failed to create queries folder: %w", err)
		}
		if err := os.Mkdir(txnDir, 0o700); err != nil {
			if os.IsExist(err) {
				// Another process created it between our Lstat and Mkdir;
				// harmless, since both attempts want the same directory.
				return txnDir, nil
			}
			return "", fmt.Errorf("failed to create query transaction directory: %w", err)
		}
		// The lock file and any transaction artifact are internal
		// bookkeeping, never a query asset a capture/commit should show -
		// a "*" .gitignore here excludes the whole directory (itself
		// included) from `git status` in the project's own repo, verified
		// empirically: a .gitignore whose own directory is "*"-ignored
		// also ignores itself. Best-effort: a project that isn't a Git
		// repository, or one where this write fails, still works fine
		// without it.
		_ = os.WriteFile(path.Join(txnDir, ".gitignore"), []byte("*\n"), 0o600)
		return txnDir, nil
	default:
		return "", err
	}
}

// queryTxnDirHasRecoveryContent reports whether txnDir holds a journal or
// a staged (not yet journaled) file - the artifacts an interrupted
// transaction leaves behind, and the only things an over-permissioned
// transaction directory could let a tampering actor have altered. It only
// checks existence (Lstat), never trusts what it finds: readJournal and
// ensureInstalled still apply their own permission/hash checks to
// anything actually read once ensureQueryTxnDir decides the directory
// (now at, or repaired to, 0700) is safe to use. It deliberately ignores
// the "lock" and ".gitignore" files ensureQueryTxnDir/withQueryLock write
// themselves - neither is a recovery artifact, so their presence alone
// must not block the permission repair below.
func queryTxnDirHasRecoveryContent(txnDir string) (bool, error) {
	for _, name := range []string{queryTxnJournalFile, queryTxnStagedJSON, queryTxnStagedBody} {
		if _, err := os.Lstat(path.Join(txnDir, name)); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, err
		}
	}
	return false, nil
}
