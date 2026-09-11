//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Review SF-A: a target locked in Finder (uchg), made append-only (uappnd),
// or a folder made append-only, passed every check before the commit and
// then failed the install, which wedged every later read and write. Each is
// now refused before the commit, as an ordinary error on that one write.

func setFileFlags(t *testing.T, p string, flags int) {
	t.Helper()
	if err := syscall.Chflags(p, flags); err != nil {
		t.Skipf("cannot set file flags on %s: %v", p, err)
	}
	t.Cleanup(func() { _ = syscall.Chflags(p, 0) })
}

func TestLockedTargets_AreRefusedBeforeTheCommit(t *testing.T) {
	saveQ := func(ps fsProjectStore, _ datatug.QueryRevision) error {
		q := dtqlQuery("q", "", "T1")
		return ps.SaveQuery(context.Background(), &q)
	}
	for _, tc := range []struct {
		name, target string
		flags        int
		write        func(fsProjectStore, datatug.QueryRevision) error
	}{
		{"uchg body, PutQuery", "q.query.dtql", bsdUserImmutable, putQ("t1", "T1", datatug.QueryTypeDTQL)},
		{"uchg metadata, PutQuery", "q.query.json", bsdUserImmutable, putQ("t1", "T1", datatug.QueryTypeDTQL)},
		{"uappnd metadata, SaveQuery", "q.query.json", bsdUserAppend, saveQ},
		{"uchg stale body, type change", "q.query.dtql", bsdUserImmutable, putQ("t1", "SELECT 1", datatug.QueryTypeSQL)},
		{"uchg metadata, DeleteQuery", "q.query.json", bsdUserImmutable, func(ps fsProjectStore, _ datatug.QueryRevision) error {
			return ps.DeleteQuery(context.Background(), "q")
		}},
		{"uchg body, DeleteQueryRevision", "q.query.dtql", bsdUserImmutable, func(ps fsProjectStore, rev datatug.QueryRevision) error {
			return ps.DeleteQueryRevision(context.Background(), "q", rev)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps, queriesDir, rev := seedQ(t)
			setFileFlags(t, filepath.Join(queriesDir, tc.target), tc.flags)
			err := tc.write(ps, rev)
			requireRefusedWrite(t, err, "is locked")
			if errors.As(err, new(*queryTxnIncompleteError)) {
				t.Errorf("expected a refusal before the commit, got: %v", err)
			}
			if got, err := ps.LoadQueryRevision(context.Background(), "q"); err != nil || got.Revision != rev {
				t.Errorf("expected q unchanged at %s, got %+v, %v", rev, got, err)
			}
			assertStoreUsableAfterRefusal(t, ps, queriesDir)
		})
	}
}

func TestAppendOnlyFolders_AreRefusedBeforeTheCommit(t *testing.T) {
	for _, folder := range []string{"queries folder", "transaction directory"} {
		t.Run(folder, func(t *testing.T) {
			ps, queriesDir, rev := seedQ(t)
			dir := queriesDir
			if folder == "transaction directory" {
				dir = filepath.Join(queriesDir, reservedQueryTxnDirName)
			}
			setFileFlags(t, dir, bsdUserAppend)
			requireRefusedWrite(t, putQ("t1", "T1", datatug.QueryTypeDTQL)(ps, rev), "is locked")
			if got := txnDirEntries(t, queriesDir); got != ".gitignore lock" {
				t.Errorf("expected nothing staged or committed, got %q", got)
			}
			if q, err := ps.LoadQuery(context.Background(), "other"); err != nil || q.Text != "OTHER" {
				t.Errorf("LoadQuery(other): %v", err)
			}
			if err := syscall.Chflags(dir, 0); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err := putQ("t1", "T1", datatug.QueryTypeDTQL)(ps, rev); err != nil {
				t.Errorf("expected the write to succeed once the folder is unlocked, got: %v", err)
			}
		})
	}
}

// A target locked after a writer committed and crashed is seen by
// recovery's own checks before it touches anything: only that query is
// refused, nothing is half installed, and unlocking completes the write.
func TestTargetLockedAfterTheCommit_BlocksOnlyItsQuery(t *testing.T) {
	ctx := context.Background()
	_, queriesDir, _ := seedQ(t)
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	update := dtqlQuery("q", "", "T1")
	update.Title = "t1"
	jsonBytes, err := queryJSONBytes(update.QueryDef)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for name, data := range map[string][]byte{queryTxnStagedJSON: jsonBytes, queryTxnStagedBody: []byte("T1")} {
		if err := writeStagedFile(txnDir, name, data); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	j := queryTxnJournal{
		ID: "q", Operation: queryTxnOpPut,
		JSONFileName: "q.query.json", JSONHash: hashBytes(jsonBytes),
		BodyFileName: "q.query.dtql", BodyHash: hashBytes([]byte("T1")),
		HadPrevious: true, PrevBodyFileName: "q.query.dtql",
	}
	if err := commitQueryTransaction(queriesDir, txnDir, j); err != nil {
		t.Fatalf("unexpected error committing: %v", err)
	}
	metadata := filepath.Join(queriesDir, "q.query.json")
	setFileFlags(t, metadata, bsdUserImmutable)

	fresh := newFsProjectStore("p", filepath.Dir(queriesDir))
	_, err = fresh.LoadQueryRevision(ctx, "q")
	requireStuck(t, "LoadQueryRevision(q)", err)
	requireRefusedWrite(t, err, "is locked")
	if q, err := fresh.LoadQuery(ctx, "other"); err != nil || q.Text != "OTHER" {
		t.Errorf("LoadQuery(other): %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(queriesDir, "q.query.dtql")); string(b) != "T0" {
		t.Errorf("expected recovery to touch nothing before its checks pass, got body %q", b)
	}
	if err := syscall.Chflags(metadata, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	requireQ(t, fresh, "t1", "T1", datatug.QueryTypeDTQL)
}
