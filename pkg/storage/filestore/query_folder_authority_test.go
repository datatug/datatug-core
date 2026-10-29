package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestGuardedQueryFolderDecisionProtectsNewAndRepeatedTargets(t *testing.T) {
	denied := errors.New("serving authority revoked")
	for _, existing := range []bool{false, true} {
		for _, decision := range []string{"allow", "deny", "cancel in authorization"} {
			t.Run(fmtFolderDecision(existing, decision), func(t *testing.T) {
				s, root := newTestQueriesStore(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				// Initialize the owning lock infrastructure before the target snapshot.
				if err := s.withQueryLock(ctx, func(queryLockGuard) error { return nil }); err != nil {
					t.Fatal(err)
				}
				if existing {
					if err := s.CreateQueryFolder(ctx, "", "reports"); err != nil {
						t.Fatal(err)
					}
				}
				before := snapshotDir(t, root)
				calls := 0
				err := s.CreateQueryFolderGuarded(ctx, "", "reports", func() error {
					calls++
					if !reflect.DeepEqual(before, snapshotDir(t, root)) {
						t.Fatal("target changed before authority decision")
					}
					if decision == "deny" {
						return denied
					}
					if decision == "cancel in authorization" {
						cancel()
					}
					return nil
				})
				if calls != 1 {
					t.Fatalf("authority checked %d times, want one decision even for repeat", calls)
				}
				if decision == "allow" {
					if err != nil {
						t.Fatal(err)
					}
					for _, name := range []string{"README.md", queryFolderDeclarationFile} {
						if _, err := os.Stat(filepath.Join(root, "reports", name)); err != nil {
							t.Fatal("accepted target missing", err)
						}
					}
					return
				}
				want := denied
				if decision == "cancel in authorization" {
					want = context.Canceled
				}
				if !errors.Is(err, want) || !reflect.DeepEqual(before, snapshotDir(t, root)) {
					t.Fatalf("refused target changed or acknowledged: %v", err)
				}
			})
		}
	}
}

func fmtFolderDecision(existing bool, decision string) string {
	if existing {
		return "repeat/" + decision
	}
	return "new/" + decision
}

func TestGuardedQueryFolderRechecksAfterRealProviderLockWait(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmtFolderDecision(canceled, "provider lock"), func(t *testing.T) {
			s, root := newTestQueriesStore(t)
			txn, err := ensureQueryTxnDir(root)
			if err != nil {
				t.Fatal(err)
			}
			lock := flock.New(filepath.Join(txn, queryTxnLockFile))
			if err := lock.Lock(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = lock.Unlock() }()
			original := tryLockContext
			t.Cleanup(func() { tryLockContext = original })
			entered := make(chan struct{})
			tryLockContext = func(f *flock.Flock, ctx context.Context, delay time.Duration) (bool, error) {
				close(entered)
				return original(f, ctx, delay)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var revoked atomic.Bool
			denied := errors.New("authority replaced during provider lock wait")
			result := make(chan error, 1)
			go func() {
				result <- s.CreateQueryFolderGuarded(ctx, "", "must-not-exist", func() error {
					if revoked.Load() {
						return denied
					}
					return nil
				})
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("provider never reached held lock")
			}
			revoked.Store(true)
			want := denied
			if canceled {
				cancel()
				want = context.Canceled
			}
			if err := lock.Unlock(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-result:
				if !errors.Is(err, want) {
					t.Fatalf("wait acknowledged revoked/canceled create: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("provider did not finish after lock release")
			}
			if _, err := os.Lstat(filepath.Join(root, "must-not-exist")); !os.IsNotExist(err) {
				t.Fatal("revoked request created target/README/declaration", err)
			}
		})
	}
}

type folderCancellationOnClose struct {
	safeReadFile
	cancel context.CancelFunc
}

func (f folderCancellationOnClose) Close() error {
	err := f.safeReadFile.Close()
	f.cancel()
	return err
}

func TestGuardedQueryFolderCancellationDuringRepeatAdmissionCannotAcknowledge(t *testing.T) {
	s, root := newTestQueriesStore(t)
	if err := s.CreateQueryFolder(context.Background(), "", "reports"); err != nil {
		t.Fatal(err)
	}
	before := snapshotDir(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	open := openRegularFileForRead
	t.Cleanup(func() { openRegularFileForRead = open })
	openRegularFileForRead = func(name string) (safeReadFile, error) {
		f, err := open(name)
		if err == nil && name == filepath.Join(root, "reports", queryFolderDeclarationFile) {
			return folderCancellationOnClose{safeReadFile: f, cancel: cancel}, nil
		}
		return f, err
	}
	calls := 0
	err := s.CreateQueryFolderGuarded(ctx, "", "reports", func() error { calls++; return nil })
	if !errors.Is(err, context.Canceled) || calls != 0 || !reflect.DeepEqual(before, snapshotDir(t, root)) {
		t.Fatalf("cancellation in admission reached authority/ack or changed target: %v calls=%d", err, calls)
	}
}
