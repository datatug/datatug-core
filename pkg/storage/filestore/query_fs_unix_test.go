//go:build unix

package filestore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
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
