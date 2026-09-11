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

// queryTxnLockFile is the advisory lock file inside the transaction
// directory.
const queryTxnLockFile = "lock"

// checkQueryLockFile refuses a lock file that is not a regular file owned
// by the effective user. The lock is only ever opened and flock()ed, never
// read or written, but opening a planted symlink would create or open a
// file wherever it points (flock opens with O_CREATE), and opening a
// planted FIFO blocks. A missing lock is fine: flock creates it, and
// queryLockOpenFlags makes that open refuse a symlink - and not block on a
// FIFO - swapped in after this check. Anything else is refused rather than
// deleted and recreated: another process may hold a lock on the existing
// file, and unlinking it would let two processes lock different files. The
// lock's permission bits are not checked, since an archive round trip can
// leave it 0644 and nothing is ever read from it.
func checkQueryLockFile(lockPath string) error {
	info, exists, err := lstatRegularFile(lockPath)
	if err != nil {
		return fmt.Errorf("query store lock file: %w", err)
	}
	if exists && !fileOwnedByCurrentUser(info) {
		return fmt.Errorf("query store lock file %s is owned by another user; refusing to use it", lockPath)
	}
	return nil
}

// queryLockOpenFlags are the flags the lock file is opened with: flock's
// own defaults (create; read-only, or read-write on the platforms that can
// only take an exclusive lock on a writable descriptor) plus
// openNoFollowFlags.
func queryLockOpenFlags() int {
	flags := os.O_CREATE | os.O_RDONLY
	switch runtime.GOOS {
	case "aix", "solaris", "illumos":
		flags = os.O_CREATE | os.O_RDWR
	}
	return flags | openNoFollowFlags
}

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
	lockPath := path.Join(txnDir, queryTxnLockFile)
	if err := checkQueryLockFile(lockPath); err != nil {
		return err
	}
	fl := flock.New(lockPath, flock.SetFlag(queryLockOpenFlags()), flock.SetPermissions(0o600))
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
// existing file. One exception: if that namespace exists with permissions
// broader than 0700, even a read first repairs them (see ensureQueryTxnDir),
// and on a medium where that chmod is impossible (an immutable flag, a
// read-only mount) the read fails closed rather than trusting the directory.
//
// The unlocked path is only safe if no writer started during it (review
// S1): the first write to a project creates the namespace and then
// installs a pair, so a reader that began before the namespace existed
// could see that install half done. So after an unlocked fn, the namespace
// is checked again. If it appeared, fn's result - including any error - is
// discarded and fn runs again under the lock. That is sufficient because
// every query-pair mutation happens after the namespace is created (every
// writer goes through withQueryLock, which creates it first): if it still
// does not exist when fn has returned, no mutation overlapped fn. fn must
// therefore tolerate being called twice (a read that overwrites its
// results), which every caller's closure does. This covers LoadQuery,
// LoadQueries, LoadQueryRevision and the recursive project-tree load, which
// all read through here.
func (s fsQueriesStore) withQueryReadLock(ctx context.Context, fn func(g queryLockGuard) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.queryTxnNamespaceExists() {
		err := fn(queryLockGuard{})
		if !s.queryTxnNamespaceExists() {
			return err
		}
	}
	return s.withQueryLock(ctx, fn)
}

// queryTxnNamespaceExists reports whether the reserved transaction
// namespace exists as a real directory (Lstat) - the same condition under
// which a writer can have installed anything.
func (s fsQueriesStore) queryTxnNamespaceExists() bool {
	info, err := os.Lstat(path.Join(s.dirPath, reservedQueryTxnDirName))
	return err == nil && info.IsDir()
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
// chmod. The repair tightens the directory to 0700 first and only then
// checks it for recovery artifacts, so content planted while it was broad
// is still refused, and nothing can be planted after the chmod. Windows does not expose POSIX permission bits through
// os.FileMode (Go reports a synthetic value there), so the permission
// check only applies on the platforms where it is meaningful.
func ensureQueryTxnDir(queriesRoot string) (string, error) {
	txnDir := path.Join(queriesRoot, reservedQueryTxnDirName)
	// The queries root must itself be a real directory: a symlinked
	// "queries" would put the transaction directory, and every write,
	// wherever it points.
	if _, err := walkQueryDir(queriesRoot, "", "", false); err != nil {
		return "", err
	}
	info, err := os.Lstat(txnDir)
	switch {
	case err == nil:
		if err := vetExistingQueryTxnDir(txnDir, info); err != nil {
			return "", err
		}
		return txnDir, nil
	case os.IsNotExist(err):
		if _, err := walkQueryDir(queriesRoot, "", "", true); err != nil {
			return "", err
		}
		if err := os.Mkdir(txnDir, 0o700); err != nil {
			if os.IsExist(err) {
				// Another process created it between our Lstat and Mkdir.
				// Vet it exactly like any directory found already there.
				info, err := os.Lstat(txnDir)
				if err != nil {
					return "", err
				}
				if err := vetExistingQueryTxnDir(txnDir, info); err != nil {
					return "", err
				}
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

// vetExistingQueryTxnDir decides whether an existing ".dt-query-txn" entry
// (described by Lstat) may be used. It must be an ordinary directory - a
// symlink or anything else is refused outright - and it must be owned by
// the effective user running this process: a directory someone else
// created (an archive extracted as root keeps its original owner; a server
// running as root in a container would otherwise trust an attacker's
// directory) is refused before anything else, including the permission
// repair below, is attempted. When its permissions are broader than 0700
// it is tightened first and only then checked for recovery artifacts, so
// content planted while it was broad is still refused and nothing can be
// planted after the chmod (see ensureQueryTxnDir).
func vetExistingQueryTxnDir(txnDir string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("query transaction directory %s is a symlink; refusing to use it", txnDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("query transaction path %s is not a directory; refusing to use it", txnDir)
	}
	if !fileOwnedByCurrentUser(info) {
		return fmt.Errorf("query transaction directory %s is owned by another user; refusing to use it", txnDir)
	}
	if runtime.GOOS == "windows" || info.Mode().Perm()&^0o700 == 0 {
		return nil
	}
	// Tighten first, then look. Once the directory is 0700 no other user
	// can add anything to it, so an emptiness check made after the chmod
	// cannot be raced by an injected journal or staged file; checking first
	// and tightening second left exactly that window open to anyone who
	// could write to the broad directory. The chmod goes through a
	// descriptor opened without following a symlink and proven to be the
	// directory Lstat described (chmodDirNoFollow), so a symlink swapped in
	// after the Lstat cannot redirect it to its target (review N-e).
	if err := chmodDirNoFollow(txnDir, info, 0o700); err != nil {
		return fmt.Errorf("failed to repair query transaction directory permissions: %w", err)
	}
	tightened, err := os.Lstat(txnDir)
	if err != nil {
		return fmt.Errorf("failed to re-inspect query transaction directory %s after repairing its permissions: %w", txnDir, err)
	}
	if tightened.Mode()&os.ModeSymlink != 0 || !tightened.IsDir() || !fileOwnedByCurrentUser(tightened) || tightened.Mode().Perm()&^0o700 != 0 {
		return fmt.Errorf("query transaction directory %s changed while its permissions were being repaired; refusing to use it", txnDir)
	}
	hasContent, err := queryTxnDirHasRecoveryContent(txnDir)
	if err != nil {
		return err
	}
	if hasContent {
		return fmt.Errorf("query transaction directory %s has overly broad permissions %v; refusing to use it", txnDir, info.Mode().Perm())
	}
	// The lock is expected to be there, so it is not recovery content, but
	// a symlink or FIFO planted in its place while the directory was broad
	// is refused here too.
	return checkQueryLockFile(path.Join(txnDir, queryTxnLockFile))
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
// must not block the permission repair. (vetExistingQueryTxnDir checks the
// lock's type and owner separately, with checkQueryLockFile.)
func queryTxnDirHasRecoveryContent(txnDir string) (bool, error) {
	for _, name := range []string{queryTxnJournalFile, queryTxnJournalTmpFile, queryTxnStagedJSON, queryTxnStagedBody} {
		if _, err := os.Lstat(path.Join(txnDir, name)); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, err
		}
	}
	return false, nil
}
