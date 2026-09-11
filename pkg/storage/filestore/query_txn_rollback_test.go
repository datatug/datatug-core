package filestore

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Review SF-B: the recovery advice must never lead to a torn pair. It is
// fix-forward only, and the one way back - the committing writer's own
// rollback - is taken only while nothing at the query's location has
// changed. These tests make one install step fail (queryTargetRename or
// queryTargetRemove) and check both halves.

// failQueryTargetStep makes every query-location rename onto (op "rename")
// or removal of (op "remove") a file named name fail with EACCES, until the
// returned restore is called or the test ends.
func failQueryTargetStep(t *testing.T, op, name string) (restore func()) {
	t.Helper()
	origRename, origRemove := queryTargetRename, queryTargetRemove
	restore = func() { queryTargetRename, queryTargetRemove = origRename, origRemove }
	t.Cleanup(restore)
	switch op {
	case "rename":
		queryTargetRename = func(oldPath, newPath string) error {
			if filepath.Base(newPath) == name {
				return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: fs.ErrPermission}
			}
			return origRename(oldPath, newPath)
		}
	case "remove":
		queryTargetRemove = func(p string) error {
			if filepath.Base(p) == name {
				return &os.PathError{Op: "remove", Path: p, Err: fs.ErrPermission}
			}
			return origRemove(p)
		}
	default:
		t.Fatalf("unknown op %q", op)
	}
	return restore
}

// seedQ writes query "q" (title t0, DTQL text T0) next to "other".
func seedQ(t *testing.T) (fsProjectStore, string, datatug.QueryRevision) {
	t.Helper()
	ps, queriesDir := newPreflightProject(t)
	q := dtqlQuery("q", "", "T0")
	q.Title = "t0"
	stored, err := ps.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatalf("unexpected error seeding q: %v", err)
	}
	return ps, queriesDir, stored.Revision
}

func putQ(title, text string, typ datatug.QueryType) func(fsProjectStore, datatug.QueryRevision) error {
	return func(ps fsProjectStore, rev datatug.QueryRevision) error {
		q := dtqlQuery("q", "", text)
		q.Title, q.Type = title, typ
		_, err := ps.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfMatch: rev})
		return err
	}
}

func TestWriter_RollsBackWhenItsFirstInstallStepFails(t *testing.T) {
	for _, tc := range []struct {
		name, op, target string
		write            func(fsProjectStore, datatug.QueryRevision) error
	}{
		{"PutQuery, body rename", "rename", "q.query.dtql", putQ("t1", "T1", datatug.QueryTypeDTQL)},
		{"SaveQuery, body rename", "rename", "q.query.dtql", func(ps fsProjectStore, _ datatug.QueryRevision) error {
			q := dtqlQuery("q", "", "T1")
			q.Title = "t1"
			return ps.SaveQuery(context.Background(), &q)
		}},
		{"type change, stale body removal", "remove", "q.query.dtql", putQ("t1", "SELECT 1", datatug.QueryTypeSQL)},
		{"DeleteQuery, body removal", "remove", "q.query.dtql", func(ps fsProjectStore, _ datatug.QueryRevision) error {
			return ps.DeleteQuery(context.Background(), "q")
		}},
		{"DeleteQueryRevision, body removal", "remove", "q.query.dtql", func(ps fsProjectStore, rev datatug.QueryRevision) error {
			return ps.DeleteQueryRevision(context.Background(), "q", rev)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps, queriesDir, rev := seedQ(t)
			failQueryTargetStep(t, tc.op, tc.target)
			err := tc.write(ps, rev)
			requireRefusedWrite(t, err, "nothing was changed")
			if errors.As(err, new(*queryTxnIncompleteError)) {
				t.Errorf("expected an ordinary error after a rollback, got the incomplete-transaction error: %v", err)
			}
			if !errors.Is(err, fs.ErrPermission) {
				t.Errorf("expected the cause to be kept, got: %v", err)
			}
			got, err := ps.LoadQueryRevision(context.Background(), "q")
			if err != nil {
				t.Fatalf("expected the old pair intact, got: %v", err)
			}
			if got.Revision != rev || got.Query.Title != "t0" || got.Query.Text != "T0" || got.Query.Type != datatug.QueryTypeDTQL {
				t.Errorf("expected the old pair t0/T0 at %s, got %q/%q (%s) at %s", rev, got.Query.Title, got.Query.Text, got.Query.Type, got.Revision)
			}
			assertStoreUsableAfterRefusal(t, ps, queriesDir)
		})
	}
}

func TestWriter_NeverRollsBackOnceTheInstallChangedSomething(t *testing.T) {
	for _, tc := range []struct {
		name, op, target string
		write            func(fsProjectStore, datatug.QueryRevision) error
		check            func(t *testing.T, ps fsProjectStore)
	}{
		{"PutQuery, metadata rename after the body", "rename", "q.query.json", putQ("t1", "T1", datatug.QueryTypeDTQL),
			func(t *testing.T, ps fsProjectStore) { requireQ(t, ps, "t1", "T1", datatug.QueryTypeDTQL) }},
		{"type change, body rename after the stale removal", "rename", "q.query.sql", putQ("t1", "SELECT 1", datatug.QueryTypeSQL),
			func(t *testing.T, ps fsProjectStore) { requireQ(t, ps, "t1", "SELECT 1", datatug.QueryTypeSQL) }},
		{"DeleteQuery, metadata removal after the body", "remove", "q.query.json", func(ps fsProjectStore, _ datatug.QueryRevision) error {
			return ps.DeleteQuery(context.Background(), "q")
		}, func(t *testing.T, ps fsProjectStore) {
			if _, err := ps.LoadQueryRevision(context.Background(), "q"); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("expected the delete completed, got: %v", err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps, queriesDir, rev := seedQ(t)
			restore := failQueryTargetStep(t, tc.op, tc.target)
			err := tc.write(ps, rev)
			var incomplete *queryTxnIncompleteError
			if !errors.As(err, &incomplete) {
				t.Fatalf("expected the incomplete-transaction error once part of the pair was installed, got: %v", err)
			}
			journal := filepath.Join(queriesDir, reservedQueryTxnDirName, queryTxnJournalFile)
			msg := filepath.ToSlash(err.Error())
			for _, want := range []string{"next access", "Do not remove " + filepath.ToSlash(journal), tc.target} {
				if !strings.Contains(msg, want) {
					t.Errorf("expected the advice to mention %q, got: %v", want, err)
				}
			}
			if strings.Contains(msg, "abandon") {
				t.Errorf("expected no abandon advice, got: %v", err)
			}
			if !fileExistsAt(journal) {
				t.Fatal("expected the journal kept for recovery to complete forward")
			}
			// The user fixes the entry; the next access completes the write.
			restore()
			tc.check(t, ps)
			if fileExistsAt(journal) {
				t.Error("expected the journal removed once the write completed")
			}
		})
	}
}

func requireQ(t *testing.T, ps fsProjectStore, title, text string, typ datatug.QueryType) {
	t.Helper()
	got, err := ps.LoadQueryRevision(context.Background(), "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Query.Title != title || got.Query.Text != text || got.Query.Type != typ {
		t.Errorf("expected %q/%q (%s), got %q/%q (%s)", title, text, typ, got.Query.Title, got.Query.Text, got.Query.Type)
	}
}

// queryTxnUntouched judges only from what is on disk, and anything it
// cannot prove untouched counts as touched.
func TestQueryTxnUntouched_JudgesFromWhatIsOnDisk(t *testing.T) {
	payload := []byte("x")
	put := queryTxnJournal{
		ID: "q", Operation: queryTxnOpPut,
		JSONFileName: "q.query.json", JSONHash: hashBytes(payload),
		BodyFileName: "q.query.sql", BodyHash: hashBytes(payload),
		HadPrevious: true, PrevBodyFileName: "q.query.dtql",
	}
	p := plantTxn(t, put, payload, payload)
	stale := filepath.Join(p.queriesDir, "q.query.dtql")
	writeFile0600(t, stale, []byte("old"))
	if !queryTxnUntouched(p.queriesDir, p.txnDir, put) {
		t.Error("put: expected untouched with both staged files and the stale body present")
	}
	if err := os.Remove(stale); err != nil {
		t.Fatal(err)
	}
	if queryTxnUntouched(p.queriesDir, p.txnDir, put) {
		t.Error("put: expected touched once the stale body is gone")
	}
	writeFile0600(t, stale, []byte("old"))
	if err := os.Remove(filepath.Join(p.txnDir, queryTxnStagedBody)); err != nil {
		t.Fatal(err)
	}
	if queryTxnUntouched(p.queriesDir, p.txnDir, put) {
		t.Error("put: expected touched once the staged body has left the transaction directory")
	}

	del := queryTxnJournal{ID: "d", Operation: queryTxnOpDelete, JSONFileName: "d.query.json", BodyFileName: "d.query.dtql"}
	d := plantTxn(t, del, nil, nil)
	writeFile0600(t, filepath.Join(d.queriesDir, "d.query.json"), []byte("{}"))
	writeFile0600(t, filepath.Join(d.queriesDir, "d.query.dtql"), []byte("b"))
	if !queryTxnUntouched(d.queriesDir, d.txnDir, del) {
		t.Error("delete: expected untouched with both files present")
	}
	if err := os.Remove(filepath.Join(d.queriesDir, "d.query.dtql")); err != nil {
		t.Fatal(err)
	}
	if queryTxnUntouched(d.queriesDir, d.txnDir, del) {
		t.Error("delete: expected touched once the body is removed")
	}
	inFolder := del
	inFolder.FolderPath = "missing"
	if queryTxnUntouched(d.queriesDir, d.txnDir, inFolder) {
		t.Error("delete: expected a folder that cannot be resolved to count as touched")
	}
	if queryTxnUntouched(d.queriesDir, d.txnDir, queryTxnJournal{Operation: "bogus"}) {
		t.Error("expected an unknown operation to count as touched")
	}
}
