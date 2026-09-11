package filestore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Review SF-A: whatever the checks before the commit cannot see must not
// wedge the store. A committed write that cannot complete keeps its slot;
// only the query it names is refused - its reads, its writes and its
// folder's listing, under any spelling - and every other query is read and
// written as usual, in the same store and in a freshly opened one. The
// named query is never served torn, and the first access after the entry
// is fixed completes the write.

func requireStuck(t *testing.T, label string, err error) {
	t.Helper()
	var incomplete *queryTxnIncompleteError
	if !errors.As(err, &incomplete) {
		t.Errorf("%s: expected the named query refused with the incomplete-transaction error, got: %v", label, err)
	}
}

func txnDirEntries(t *testing.T, queriesDir string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(queriesDir, reservedQueryTxnDirName))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return strings.Join(names, " ")
}

func TestStuckWrite_RefusesOnlyTheNamedQuery(t *testing.T) {
	ctx := context.Background()
	ps, queriesDir, rev := seedQ(t)
	sub := dtqlQuery("x", "sub", "SUB")
	if _, err := ps.PutQuery(ctx, &sub, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The body installs, then the metadata rename fails: half installed.
	restore := failQueryTargetStep(t, "rename", "q.query.json")
	requireStuck(t, "the writer", putQ("t1", "T1", datatug.QueryTypeDTQL)(ps, rev))

	fresh := newFsProjectStore("p", filepath.Dir(queriesDir))
	for name, store := range map[string]fsProjectStore{"same store": ps, "fresh store": fresh} {
		t.Run(name, func(t *testing.T) {
			if q, err := store.LoadQuery(ctx, "other"); err != nil || q.Text != "OTHER" {
				t.Errorf("LoadQuery(other): %v", err)
			}
			if _, err := store.LoadQueryRevision(ctx, "other"); err != nil {
				t.Errorf("LoadQueryRevision(other): %v", err)
			}
			if folder, err := store.LoadQueries(ctx, "sub"); err != nil || len(folder.Items) != 1 {
				t.Errorf("LoadQueries(sub): %v", err)
			}
			third := dtqlQuery("third", "", "THIRD")
			if _, err := store.PutQuery(ctx, &third, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
				t.Errorf("PutQuery(third): %v", err)
			}
			if got, err := store.LoadQueryRevision(ctx, "third"); err != nil || got.Query.Text != "THIRD" {
				t.Errorf("LoadQueryRevision(third): %v", err)
			}
			if err := store.DeleteQuery(ctx, "third"); err != nil {
				t.Errorf("DeleteQuery(third): %v", err)
			}

			_, err := store.LoadQueryRevision(ctx, "q")
			requireStuck(t, "LoadQueryRevision(q)", err)
			_, err = store.LoadQuery(ctx, "q")
			requireStuck(t, "LoadQuery(q)", err)
			_, err = store.LoadQuery(ctx, "Q")
			requireStuck(t, "LoadQuery(Q), another spelling", err)
			requireStuck(t, "PutQuery(q)", putQ("t2", "T2", datatug.QueryTypeDTQL)(store, rev))
			again := dtqlQuery("q", "", "T3")
			requireStuck(t, "SaveQuery(q)", store.SaveQuery(ctx, &again))
			requireStuck(t, "DeleteQuery(q)", store.DeleteQuery(ctx, "q"))
			requireStuck(t, "DeleteQueryRevision(q)", store.DeleteQueryRevision(ctx, "q", rev))
			_, err = store.LoadQueries(ctx, "")
			requireStuck(t, "LoadQueries of q's folder", err)
			if _, err := store.LoadProject(ctx); err == nil || !strings.Contains(err.Error(), "cannot be completed yet") {
				t.Errorf("LoadProject: expected the stuck query's folder to refuse the tree, got: %v", err)
			}
		})
	}

	// The user fixes the entry; the next access completes the write.
	restore()
	requireQ(t, ps, "t1", "T1", datatug.QueryTypeDTQL)
	if _, err := ps.LoadQueries(ctx, ""); err != nil {
		t.Errorf("LoadQueries after the fix: %v", err)
	}
	if _, err := ps.LoadProject(ctx); err != nil {
		t.Errorf("LoadProject after the fix: %v", err)
	}
	if got := txnDirEntries(t, queriesDir); got != ".gitignore lock" {
		t.Errorf("expected only .gitignore and lock left in the transaction directory, got %q", got)
	}
}

// A process that committed and crashed before installing leaves a journal
// recovery cannot complete: recovery never rolls it back, and a freshly
// opened store refuses only that query.
func TestStuckWrite_CommittedBeforeACrash_BlocksOnlyItsQuery(t *testing.T) {
	ctx := context.Background()
	_, queriesDir := newPreflightProject(t)
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	n := dtqlQuery("n", "", "NEW")
	jsonBytes, err := queryJSONBytes(n.QueryDef)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for name, data := range map[string][]byte{queryTxnStagedJSON: jsonBytes, queryTxnStagedBody: []byte(n.Text)} {
		if err := writeStagedFile(txnDir, name, data); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	j := queryTxnJournal{
		ID: "n", Operation: queryTxnOpPut,
		JSONFileName: "n.query.json", JSONHash: hashBytes(jsonBytes),
		BodyFileName: "n.query.dtql", BodyHash: hashBytes([]byte(n.Text)),
	}
	if err := commitQueryTransaction(queriesDir, txnDir, j); err != nil {
		t.Fatalf("unexpected error committing: %v", err)
	}
	restore := failQueryTargetStep(t, "rename", "n.query.dtql")

	fresh := newFsProjectStore("p", filepath.Dir(queriesDir))
	if q, err := fresh.LoadQuery(ctx, "other"); err != nil || q.Text != "OTHER" {
		t.Errorf("LoadQuery(other): %v", err)
	}
	third := dtqlQuery("third", "", "THIRD")
	if err := fresh.SaveQuery(ctx, &third); err != nil {
		t.Errorf("SaveQuery(third): %v", err)
	}
	_, err = fresh.LoadQueryRevision(ctx, "n")
	requireStuck(t, "LoadQueryRevision(n)", err)
	competing := dtqlQuery("n", "", "COMPETING")
	_, err = fresh.PutQuery(ctx, &competing, datatug.QueryWriteCondition{IfNoneMatch: true})
	requireStuck(t, "PutQuery(n)", err)
	if !fileExistsAt(filepath.Join(txnDir, queryTxnJournalFile)) {
		t.Fatal("expected recovery never to roll back a committed transaction")
	}

	restore()
	got, err := fresh.LoadQueryRevision(ctx, "n")
	if err != nil || got.Query.Text != "NEW" {
		t.Fatalf("expected the committed write completed once the entry is fixed, got %+v, %v", got, err)
	}
}

func TestStuckWrites_WhenEverySlotIsHeld_RefuseWritesButNotReads(t *testing.T) {
	ctx := context.Background()
	ps, _ := newPreflightProject(t)
	revs := map[string]datatug.QueryRevision{}
	for i := 0; i < queryTxnSlotCount; i++ {
		id := fmt.Sprintf("q%d", i)
		q := dtqlQuery(id, "", "V0")
		stored, err := ps.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
		if err != nil {
			t.Fatalf("unexpected error seeding %s: %v", id, err)
		}
		revs[id] = stored.Revision
	}
	locked := regexp.MustCompile(`^q\d\.query\.json$`)
	orig := queryTargetRename
	t.Cleanup(func() { queryTargetRename = orig })
	queryTargetRename = func(oldPath, newPath string) error {
		if locked.MatchString(filepath.Base(newPath)) {
			return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: fs.ErrPermission}
		}
		return orig(oldPath, newPath)
	}
	for id, rev := range revs {
		q := dtqlQuery(id, "", "V1")
		q.Title = "V1" // the metadata changes too, so it is replaced after the body
		_, err := ps.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfMatch: rev})
		requireStuck(t, "PutQuery("+id+")", err)
	}
	extra := dtqlQuery("extra", "", "E")
	_, err := ps.PutQuery(ctx, &extra, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err == nil || !strings.Contains(err.Error(), "slots") {
		t.Errorf("expected a write refused while every slot is held, got: %v", err)
	}
	requireStuck(t, "the refusal carries the stuck writes' errors", err)
	if q, err := ps.LoadQuery(ctx, "other"); err != nil || q.Text != "OTHER" {
		t.Errorf("LoadQuery(other) with every slot held: %v", err)
	}

	queryTargetRename = orig
	if _, err := ps.PutQuery(ctx, &extra, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Errorf("PutQuery(extra) after the fix: %v", err)
	}
	for id := range revs {
		if got, err := ps.LoadQueryRevision(ctx, id); err != nil || got.Query.Text != "V1" {
			t.Errorf("%s: expected the stuck write completed, got %+v, %v", id, got, err)
		}
	}
}

func TestQueryNameKey_MatchesEverySpellingAFileSystemMayMerge(t *testing.T) {
	// Every pair a supported file system may resolve to one file shares a
	// key, so no spelling can slip past a stuck write. The APFS and HFS+
	// notes were measured on this machine (/private/tmp and a scratch HFS+
	// volume); the NTFS and ext4-casefold ones follow their published
	// tables - NTFS's $UpCase and Unicode simple case folding. Where only
	// one file system merges a pair the key still merges it: the key may be
	// coarser than the file system underneath it, never finer.
	for _, tc := range []struct{ why, a, b string }{
		{"ASCII case: APFS, HFS+, NTFS, ext4", "q", "Q"},
		{"ASCII case, longer", "Customer-Invoices", "customer-invoices"},
		{"composed and decomposed e-acute: APFS, HFS+", "caf\u00e9", "cafe\u0301"},
		{"Kelvin sign: APFS", "\u212aq", "kq"},
		{"long s: APFS", "\u017fum", "sum"},
		{"angstrom sign: APFS", "\u212bx", "\u00c5x"},
		{"dotless i: NTFS $UpCase folds it onto I", "\u0131d", "id"},
		{"dotted capital I: merged onto the same letter", "\u0130d", "id"},
		{"zero-width non-joiner: HFS+", "rev\u200cenue", "revenue"},
		{"byte-order mark: HFS+", "rev\ufeffenue", "revenue"},
		{"inhibit symmetric swapping: HFS+", "rev\u206aenue", "revenue"},
		{"zero-width space", "rev\u200benue", "revenue"},
		{"soft hyphen", "rev\u00adenue", "revenue"},
		{"combining grapheme joiner", "rev\u034fenue", "revenue"},
		{"variation selector", "revenue\ufe00", "revenue"},
		{"language tag", "revenue\U000e0001", "revenue"},
	} {
		if queryNameKey(tc.a) != queryNameKey(tc.b) {
			t.Errorf("%s: expected %q and %q to share a key, got %q and %q",
				tc.why, tc.a, tc.b, queryNameKey(tc.a), queryNameKey(tc.b))
		}
	}
	// Names no supported file system merges keep distinct keys, so a stuck
	// write never refuses half the project.
	for _, pair := range [][2]string{{"q", "q2"}, {"a", "b"}, {"caf\u00e9", "cafe"}, {"revenue", "revenues"}} {
		if queryNameKey(pair[0]) == queryNameKey(pair[1]) {
			t.Errorf("expected %q and %q to have different keys, both got %q", pair[0], pair[1], queryNameKey(pair[0]))
		}
	}
	if queryPathKey("Sub/Folder") != queryPathKey("sub/folder") || queryPathKey("a/b") == queryPathKey("a/c") || queryPathKey("") != "" {
		t.Error("expected queryPathKey to apply queryNameKey per segment")
	}
	if queryPathKey("re\u200cf/sub") != queryPathKey("ref/sub") {
		t.Error("expected queryPathKey to drop default-ignorable code points per segment")
	}
}

// A slot that is not the store's own directory fails closed, like the
// transaction directory itself; an empty slot left behind is removed.
func TestQueryTxnSlots_AreVettedAndTidied(t *testing.T) {
	skipSymlinksOnWindows(t)
	ctx := context.Background()
	ps, queriesDir := newPreflightProject(t)
	slot := filepath.Join(queriesDir, reservedQueryTxnDirName, "slot-1")
	if err := os.Symlink(t.TempDir(), slot); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := ps.LoadQuery(ctx, "other"); err == nil || !strings.Contains(err.Error(), "slot") {
		t.Errorf("expected a symlinked slot to be refused, got: %v", err)
	}
	if err := os.Remove(slot); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		if err := os.Mkdir(slot, 0o755); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := os.Chmod(slot, 0o755); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := ps.LoadQuery(ctx, "other"); err == nil || !strings.Contains(err.Error(), "overly broad") {
			t.Errorf("expected a broad slot to be refused, got: %v", err)
		}
		if err := os.Chmod(slot, 0o700); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := ps.LoadQuery(ctx, "other"); err != nil {
			t.Errorf("expected an empty 0700 slot to be accepted, got: %v", err)
		}
		if fileExistsAt(slot) {
			t.Error("expected recovery to remove an empty slot")
		}
	}
}

func TestEnsureQueryTxnDir_TreatsASlotAsRecoveryContent(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permission bits and an unprivileged user")
	}
	queriesDir := filepath.Join(t.TempDir(), "queries")
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	if err := os.MkdirAll(filepath.Join(txnDir, "slot-1"), 0o700); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.Chmod(txnDir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := ensureQueryTxnDir(queriesDir); err == nil || !strings.Contains(err.Error(), "overly broad") {
		t.Errorf("expected a broad transaction directory holding a slot to be refused, got: %v", err)
	}
}
