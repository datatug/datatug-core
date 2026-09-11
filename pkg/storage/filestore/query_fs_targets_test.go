package filestore

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Review S2 regression tests: every reader this branch routes - the
// revisioned LoadQueryRevision (readCurrentQueryPair), recovery
// (ensureInstalled), and the legacy LoadQuery/LoadQueries/project-tree
// loaders - reads a query file only when it is a regular file within
// maxQueryFileSize, never through a symlink, FIFO or device; and a legacy
// read or CreateQueryFolder can no longer address anything outside the
// queries root.

func skipSymlinksOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows CI")
	}
}

// queryReadErrors runs every query-store reader against id and returns
// their errors by name.
func queryReadErrors(store fsQueriesStore, id string) map[string]error {
	ctx := context.Background()
	_, revErr := store.LoadQueryRevision(ctx, id)
	_, oneErr := store.LoadQuery(ctx, id)
	_, listErr := store.LoadQueries(ctx, "")
	_, treeErr := store.loadQueriesTree(ctx, "")
	return map[string]error{"LoadQueryRevision": revErr, "LoadQuery": oneErr, "LoadQueries": listErr, "loadQueriesTree": treeErr}
}

func TestQueryReads_RefuseASymlinkedTargetFile(t *testing.T) {
	skipSymlinksOnWindows(t)
	for _, tc := range []struct{ name, file, secret string }{
		{"symlinked body (symread)", "q.query.dtql", "AWS_SECRET=abc123"},
		{"symlinked json (symread)", "q.query.json", `{"title":"secret-json","type":"DTQL"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, queriesDir := newTestQueriesStore(t)
			ctx := context.Background()
			q := dtqlQuery("q", "", "B0")
			if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
				t.Fatalf("unexpected error creating: %v", err)
			}
			secret := filepath.Join(t.TempDir(), "secret")
			if err := os.WriteFile(secret, []byte(tc.secret), 0o600); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			target := filepath.Join(queriesDir, tc.file)
			if err := os.Remove(target); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err := os.Symlink(secret, target); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for reader, err := range queryReadErrors(store, "q") {
				if err == nil || !strings.Contains(err.Error(), "symlink") {
					t.Errorf("%s: expected a symlinked target to be refused, got: %v", reader, err)
				}
			}
			replacement := dtqlQuery("q", "", "B1")
			if err := store.SaveQuery(ctx, &replacement); err == nil {
				t.Error("SaveQuery: expected a write over a symlinked target to be refused")
			}
			if err := store.DeleteQuery(ctx, "q"); err == nil {
				t.Error("DeleteQuery: expected a delete of a pair with a symlinked target to be refused")
			}
			if b, err := os.ReadFile(secret); err != nil || string(b) != tc.secret {
				t.Errorf("expected the symlink target untouched, got %q (err %v)", b, err)
			}
			if info, err := os.Lstat(target); err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Errorf("expected the planted symlink left in place, got %v (err %v)", info, err)
			}
		})
	}
}

func TestQueryReads_RefuseADirectoryAtATargetFileName(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	q := dtqlQuery("q", "", "B0")
	if _, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}
	body := filepath.Join(queriesDir, "q.query.dtql")
	if err := os.Remove(body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.Mkdir(body, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.Background()
	if _, err := store.LoadQueryRevision(ctx, "q"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("LoadQueryRevision: expected a directory body to be refused, got: %v", err)
	}
	if _, err := store.LoadQuery(ctx, "q"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("LoadQuery: expected a directory body to be refused, got: %v", err)
	}
}

func TestQueryReads_RefuseATargetFileOverTheCap(t *testing.T) {
	for _, file := range []string{"q.query.dtql", "q.query.json"} {
		t.Run(file, func(t *testing.T) {
			store, queriesDir := newTestQueriesStore(t)
			q := dtqlQuery("q", "", "B0")
			if _, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
				t.Fatalf("unexpected error creating: %v", err)
			}
			// A sparse file: over the cap without writing 16 MiB.
			if err := os.Truncate(filepath.Join(queriesDir, file), maxQueryFileSize+1); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for reader, err := range queryReadErrors(store, "q") {
				if err == nil || !strings.Contains(err.Error(), "limit") {
					t.Errorf("%s: expected a file over the cap to be refused, got: %v", reader, err)
				}
			}
		})
	}
}

func TestRecovery_RefusesASymlinkedFinalFileAndTouchesNothing(t *testing.T) {
	skipSymlinksOnWindows(t)
	payload := []byte("body")
	p := plantTxn(t, queryTxnJournal{
		ID: "x", Operation: queryTxnOpPut,
		JSONFileName: "x.query.json", JSONHash: hashBytes(payload),
		BodyFileName: "x.query.dtql", BodyHash: hashBytes(payload),
	}, payload, payload)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("KEEP"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(p.queriesDir, "x.query.dtql")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	before := snapshotTxnArtifacts(t, p.txnDir)
	queriesBefore := snapshotDir(t, p.queriesDir)

	if err := completeQueryTransaction(p.queriesDir, p.txnDir); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected a symlinked target to refuse the transaction, got: %v", err)
	}
	assertSameSnapshot(t, "transaction directory", before, snapshotTxnArtifacts(t, p.txnDir))
	assertSameSnapshot(t, "queries root", queriesBefore, snapshotDir(t, p.queriesDir))
	if b, _ := os.ReadFile(outside); string(b) != "KEEP" {
		t.Errorf("expected the symlink target untouched, got %q", b)
	}
}

func TestRecovery_RefusesToDeleteADirectoryAndTouchesNothing(t *testing.T) {
	p := plantTxn(t, queryTxnJournal{ID: "x", Operation: queryTxnOpDelete, JSONFileName: "x.query.json", BodyFileName: "x.query.dtql"}, nil, nil)
	if err := os.MkdirAll(filepath.Join(p.queriesDir, "x.query.json", "child"), 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(p.queriesDir, "x.query.dtql"), []byte("body"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	queriesBefore := snapshotDir(t, p.queriesDir)
	if err := completeQueryTransaction(p.queriesDir, p.txnDir); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("expected a directory at a target name to refuse the delete, got: %v", err)
	}
	assertSameSnapshot(t, "queries root", queriesBefore, snapshotDir(t, p.queriesDir))
}

func TestLegacyQueryReads_CannotLeaveTheQueriesRoot(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	projectDir := filepath.Dir(queriesDir)
	victimDir := filepath.Join(projectDir, "victim")
	if err := os.MkdirAll(victimDir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(victimDir, "x.query.json"), []byte(`{"title":"SECRET-JSON","type":"SQL"}`), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(victimDir, "x.query.sql"), []byte("SECRET-BODY"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(queriesDir, "customers"), 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(queriesDir, "customers", "c1.query.json"), []byte(`{"title":"C1","type":"SQL"}`), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.Background()

	for _, id := range []string{"../victim/x", "customers/../../victim/x", "/" + filepath.ToSlash(victimDir) + "/x", `..\victim\x`, ".."} {
		if q, err := store.LoadQuery(ctx, id); !datatug.IsInvalidQueryLocation(err) {
			t.Errorf("LoadQuery(%q): expected InvalidQueryLocationError, got %v (query %+v)", id, err, q)
		}
	}
	for _, folder := range []string{"../victim", "customers/../../victim", filepath.ToSlash(victimDir), "..", reservedQueryTxnDirName} {
		if f, err := store.LoadQueries(ctx, folder); !datatug.IsInvalidQueryLocation(err) {
			t.Errorf("LoadQueries(%q): expected InvalidQueryLocationError, got %v (folder %+v)", folder, err, f)
		}
	}

	// What path.Join used to tolerate harmlessly still works.
	for _, folder := range []string{"customers", "customers/", "./customers", "customers/.", "x/../customers"} {
		f, err := store.LoadQueries(ctx, folder)
		if err != nil || len(f.Items) != 1 || f.Items[0].ID != "c1" {
			t.Errorf("LoadQueries(%q): expected c1, got %+v (err %v)", folder, f, err)
		}
	}
	for _, folder := range []string{"", "."} {
		if _, err := store.LoadQueries(ctx, folder); err != nil {
			t.Errorf("LoadQueries(%q): expected the root to load, got %v", folder, err)
		}
	}
	if q, err := store.LoadQuery(ctx, "customers/c1"); err != nil || q.Title != "C1" {
		t.Errorf("LoadQuery(customers/c1): expected C1, got %+v (err %v)", q, err)
	}

	t.Run("symlinked folder", func(t *testing.T) {
		skipSymlinksOnWindows(t)
		if err := os.Symlink(victimDir, filepath.Join(queriesDir, "link")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := store.LoadQueries(ctx, "link"); !datatug.IsInvalidQueryLocation(err) {
			t.Errorf("LoadQueries(link): expected InvalidQueryLocationError, got %v", err)
		}
		if _, err := store.LoadQuery(ctx, "link/x"); !datatug.IsInvalidQueryLocation(err) {
			t.Errorf("LoadQuery(link/x): expected InvalidQueryLocationError, got %v", err)
		}
		tree, err := store.loadQueriesTree(ctx, "")
		if err != nil {
			t.Fatalf("loadQueriesTree: unexpected error: %v", err)
		}
		for _, f := range tree.Folders {
			if f.ID == "link" {
				t.Error("loadQueriesTree: expected a symlinked folder to be skipped")
			}
		}
	})
}

func TestCreateQueryFolder_StaysInsideTheQueriesRoot(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	projectDir := filepath.Dir(queriesDir)
	ctx := context.Background()

	for _, tc := range []struct{ parent, name string }{
		{"..", "escape"},
		{"", ".."},
		{"a/../..", "escape"},
		{"/abs", "escape"},
		{"", "a/b"},
		{"", ".hidden"},
		{"", reservedQueryTxnDirName},
	} {
		if err := store.CreateQueryFolder(ctx, tc.parent, tc.name); !datatug.IsInvalidQueryLocation(err) {
			t.Errorf("CreateQueryFolder(%q, %q): expected InvalidQueryLocationError, got %v", tc.parent, tc.name, err)
		}
	}
	if fileExistsAt(filepath.Join(projectDir, "escape")) || fileExistsAt(filepath.Join(filepath.Dir(projectDir), "escape")) {
		t.Error("expected nothing created outside the queries root")
	}

	if err := store.CreateQueryFolder(ctx, "", "g"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(queriesDir, "g", "README.md")); err != nil || string(b) != "# g" {
		t.Errorf("expected README.md naming the folder, got %q (err %v)", b, err)
	}
	if err := store.CreateQueryFolder(ctx, "g", "sub"); err != nil {
		t.Fatalf("unexpected error creating a nested folder: %v", err)
	}

	t.Run("symlinks", func(t *testing.T) {
		skipSymlinksOnWindows(t)
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(queriesDir, "link")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := store.CreateQueryFolder(ctx, "link", "sub"); !datatug.IsInvalidQueryLocation(err) {
			t.Errorf("CreateQueryFolder through a symlinked parent: expected InvalidQueryLocationError, got %v", err)
		}
		if entries, _ := os.ReadDir(outside); len(entries) != 0 {
			t.Errorf("expected nothing created through the symlink, found %v", entries)
		}
		// A dangling README.md symlink must not be followed to create its target.
		if err := os.MkdirAll(filepath.Join(queriesDir, "f"), 0o755); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		readmeTarget := filepath.Join(outside, "readme-target")
		if err := os.Symlink(readmeTarget, filepath.Join(queriesDir, "f", "README.md")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := store.CreateQueryFolder(ctx, "", "f"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fileExistsAt(readmeTarget) {
			t.Error("expected a dangling README.md symlink not to be followed")
		}
	})
}
