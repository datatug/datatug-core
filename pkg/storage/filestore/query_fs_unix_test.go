//go:build unix

package filestore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// fakeOwnedInfo is an os.FileInfo whose Sys() reports a chosen owner.
type fakeOwnedInfo struct {
	os.FileInfo
	uid uint32
}

func (f fakeOwnedInfo) Sys() any { return &syscall.Stat_t{Uid: f.uid} }

func TestOwnedByCurrentUser(t *testing.T) {
	f := filepath.Join(t.TempDir(), "mine")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := os.Lstat(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ownedByCurrentUser(info) {
		t.Error("expected a file this process created to be owned by the current user")
	}
	if ownedByCurrentUser(fakeOwnedInfo{FileInfo: info, uid: uint32(os.Geteuid()) + 1}) {
		t.Error("expected a file with another uid not to be owned by the current user")
	}
}

// mkfifo creates a FIFO, skipping the test where the file system cannot.
func mkfifo(t *testing.T, p string) {
	t.Helper()
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Skipf("cannot create a FIFO here: %v", err)
	}
}

// within fails the test if fn does not return within d: the FIFO cases
// below would otherwise hang the test binary instead of failing it.
func within(t *testing.T, d time.Duration, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatalf("still blocked after %v", d)
		return nil
	}
}

// TestQueryReads_RefuseAFIFOBodyWithoutHoldingTheLock is the review's fifo
// repro: a FIFO body used to block LoadQueryRevision while it held the
// store lock, so a PutQuery on a different ID timed out behind it.
func TestQueryReads_RefuseAFIFOBodyWithoutHoldingTheLock(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q", "", "B0")
	if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}
	body := filepath.Join(queriesDir, "q.query.dtql")
	if err := os.Remove(body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mkfifo(t, body)
	for reader, read := range map[string]func() error{
		"LoadQueryRevision": func() error { _, err := store.LoadQueryRevision(ctx, "q"); return err },
		"LoadQuery":         func() error { _, err := store.LoadQuery(ctx, "q"); return err },
		"LoadQueries":       func() error { _, err := store.LoadQueries(ctx, ""); return err },
	} {
		if err := within(t, 5*time.Second, read); err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("%s: expected a FIFO body to be refused, got: %v", reader, err)
		}
	}
	other := dtqlQuery("other", "", "O")
	c2, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := store.PutQuery(c2, &other, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Errorf("PutQuery on a different id: expected no wait behind a FIFO read, got: %v", err)
	}
}

func TestQueryReads_RefuseAFIFOMetadataFile(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	if err := os.MkdirAll(queriesDir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mkfifo(t, filepath.Join(queriesDir, "f.query.json"))
	ctx := context.Background()
	for reader, read := range map[string]func() error{
		"LoadQueryRevision": func() error { _, err := store.LoadQueryRevision(ctx, "f"); return err },
		"LoadQuery":         func() error { _, err := store.LoadQuery(ctx, "f"); return err },
		"LoadQueries":       func() error { _, err := store.LoadQueries(ctx, ""); return err },
	} {
		if err := within(t, 5*time.Second, read); err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("%s: expected a FIFO metadata file to be refused, got: %v", reader, err)
		}
	}
}

func TestReadRegularFileCapped_RefusesADevice(t *testing.T) {
	info, err := os.Lstat("/dev/null")
	if err != nil || info.Mode()&os.ModeDevice == 0 {
		t.Skip("/dev/null is not a device here")
	}
	if _, _, err := readRegularFileCapped("/dev/null", 16); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected a device to be refused, got: %v", err)
	}
}

// TestQueryLock_RefusesAPlantedLockFIFOWithoutBlocking is the review's
// lockplant FIFO case: a FIFO planted as "lock" kept a read blocked in
// open() for more than 5s.
func TestQueryLock_RefusesAPlantedLockFIFOWithoutBlocking(t *testing.T) {
	for _, broad := range []bool{false, true} {
		t.Run(fmt.Sprintf("broad=%v", broad), func(t *testing.T) {
			projectDir, txnDir := newTxnDirForLockTest(t, broad)
			mkfifo(t, filepath.Join(txnDir, queryTxnLockFile))
			err := within(t, 5*time.Second, func() error {
				_, err := newFsQueriesStore(projectDir).LoadQueries(context.Background(), "")
				return err
			})
			if err == nil || !strings.Contains(err.Error(), "not a regular file") {
				t.Fatalf("expected a FIFO lock to be refused, got: %v", err)
			}
		})
	}
}

// TestQueryLockOpenFlags_RefuseASymlinkAndNeverBlock proves the flags the
// lock is opened with close the window between checkQueryLockFile and the
// open: a symlink swapped in is not followed (and its target not created),
// and a FIFO swapped in does not block the open.
func TestQueryLockOpenFlags_RefuseASymlinkAndNeverBlock(t *testing.T) {
	flags := queryLockOpenFlags()
	if flags&syscall.O_NOFOLLOW == 0 || flags&syscall.O_NONBLOCK == 0 || flags&os.O_CREATE == 0 {
		t.Fatalf("expected O_CREATE|O_NOFOLLOW|O_NONBLOCK in the lock open flags, got %#x", flags)
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "lock-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f, err := os.OpenFile(link, flags, 0o600); err == nil {
		_ = f.Close()
		t.Fatal("expected opening a symlink with the lock flags to fail")
	}
	if fileExistsAt(target) {
		t.Error("expected the symlink's target not to be created")
	}
	fifo := filepath.Join(dir, "lock-fifo")
	mkfifo(t, fifo)
	if err := within(t, 5*time.Second, func() error {
		f, err := os.OpenFile(fifo, flags, 0o600)
		if err == nil {
			_ = f.Close()
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTxnArtifacts_FIFOJournalIsRefusedWithoutBlocking(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mkfifo(t, filepath.Join(txnDir, queryTxnJournalFile))
	err = within(t, 5*time.Second, func() error {
		_, err := store.LoadQueries(context.Background(), "")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected a FIFO journal to be refused, got: %v", err)
	}
}

func TestTxnArtifacts_FIFOStagedFileIsRefusedWithoutBlocking(t *testing.T) {
	payload := []byte("body")
	p := plantTxn(t, queryTxnJournal{
		ID: "x", Operation: queryTxnOpPut,
		JSONFileName: "x.query.json", JSONHash: hashBytes(payload),
		BodyFileName: "x.query.dtql", BodyHash: hashBytes(payload),
	}, payload, nil)
	mkfifo(t, filepath.Join(p.txnDir, queryTxnStagedBody))
	err := within(t, 5*time.Second, func() error {
		_, err := newFsQueriesStore(p.projectDir).LoadQueries(context.Background(), "")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected a FIFO staged file to be refused, got: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(p.queriesDir, "x.query.dtql")); err == nil {
		t.Error("expected nothing installed from a FIFO staged file")
	}
}
