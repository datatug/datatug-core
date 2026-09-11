package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Review B1 regression tests: recovery must never use a journal-supplied
// path. Each test plants a well-formed recovery state the way an archive or
// a clone delivers it (a 0700 directory holding 0600 files owned by the
// user), triggers recovery with a plain read through a fresh store, and
// proves that the victim is untouched and every planted artifact is left
// exactly as it was: fail closed, touch nothing.

type plantedTxn struct {
	projectDir, queriesDir, txnDir string
}

// plantTxn writes j (and the staged files, when non-nil) into a fresh
// project's transaction directory, bypassing every writer.
func plantTxn(t *testing.T, j queryTxnJournal, stagedJSON, stagedBody []byte) plantedTxn {
	t.Helper()
	projectDir := t.TempDir()
	p := plantedTxn{
		projectDir: projectDir,
		queriesDir: filepath.Join(projectDir, "queries"),
		txnDir:     filepath.Join(projectDir, "queries", reservedQueryTxnDirName),
	}
	if err := os.MkdirAll(p.txnDir, 0o700); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.Chmod(p.txnDir, 0o700); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stagedJSON != nil {
		writeFile0600(t, filepath.Join(p.txnDir, queryTxnStagedJSON), stagedJSON)
	}
	if stagedBody != nil {
		writeFile0600(t, filepath.Join(p.txnDir, queryTxnStagedBody), stagedBody)
	}
	b, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	writeFile0600(t, filepath.Join(p.txnDir, queryTxnJournalFile), b)
	return p
}

func writeFile0600(t *testing.T, filePath string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		t.Fatalf("unexpected error writing %s: %v", filePath, err)
	}
}

// snapshotDir records every entry directly under dir: a regular file's
// content, a symlink's target, or the entry's type.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("unexpected error reading %s: %v", dir, err)
	}
	snap := make(map[string]string, len(entries))
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		switch {
		case e.Type()&os.ModeSymlink != 0:
			target, _ := os.Readlink(p)
			snap[e.Name()] = "symlink -> " + target
		case e.Type().IsRegular():
			b, _ := os.ReadFile(p)
			snap[e.Name()] = "file: " + string(b)
		default:
			snap[e.Name()] = "type: " + e.Type().String()
		}
	}
	return snap
}

// snapshotTxnArtifacts is snapshotDir of a transaction directory without
// the "lock" file (created by lock acquisition, which precedes recovery)
// and ".gitignore": neither is a recovery artifact.
func snapshotTxnArtifacts(t *testing.T, txnDir string) map[string]string {
	t.Helper()
	snap := snapshotDir(t, txnDir)
	delete(snap, "lock")
	delete(snap, ".gitignore")
	return snap
}

func assertSameSnapshot(t *testing.T, what string, before, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Errorf("%s changed: before %v, after %v", what, before, after)
		return
	}
	for k, v := range before {
		if after[k] != v {
			t.Errorf("%s changed at %q: before %q, after %q", what, k, v, after[k])
		}
	}
}

func relFrom(t *testing.T, base, target string) string {
	t.Helper()
	r, err := filepath.Rel(base, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return filepath.ToSlash(r)
}

func TestRecovery_RefusesJournalFileNamesNotDerivedFromTheID(t *testing.T) {
	payload := []byte("echo PWNED-BY-JOURNAL\n")
	putJournal := func() queryTxnJournal {
		return queryTxnJournal{
			ID: "x", Operation: queryTxnOpPut,
			JSONFileName: "x.query.json", JSONHash: hashBytes(payload),
			BodyFileName: "x.query.dtql", BodyHash: hashBytes(payload),
		}
	}
	deleteJournal := func() queryTxnJournal {
		return queryTxnJournal{ID: "x", Operation: queryTxnOpDelete, JSONFileName: "x.query.json", BodyFileName: "x.query.dtql"}
	}
	cases := []struct {
		name string
		// journal returns the planted journal given the victim's path
		// relative to the queries root, and its own name when inside it.
		journal func(victimRel, victimName string) queryTxnJournal
		// victimInQueries places the victim inside the queries root (a
		// different query's file) instead of outside the project.
		victimInQueries bool
	}{
		{name: "put bodyFileName escapes the queries root (traverse-put)", journal: func(v, _ string) queryTxnJournal {
			j := putJournal()
			j.BodyFileName = v
			return j
		}},
		{name: "put jsonFileName escapes the queries root (traverse-jsonname)", journal: func(v, _ string) queryTxnJournal {
			j := putJournal()
			j.JSONFileName = v
			return j
		}},
		{name: "put prevBodyFileName escapes the queries root", journal: func(v, _ string) queryTxnJournal {
			j := putJournal()
			j.HadPrevious = true
			j.PrevBodyFileName = v
			return j
		}},
		{name: "delete bodyFileName escapes the queries root (traverse-del)", journal: func(v, _ string) queryTxnJournal {
			j := deleteJournal()
			j.BodyFileName = v
			return j
		}},
		{name: "delete jsonFileName escapes the queries root", journal: func(v, _ string) queryTxnJournal {
			j := deleteJournal()
			j.JSONFileName = v
			return j
		}},
		{name: "put bodyFileName is an absolute path", journal: func(_, _ string) queryTxnJournal {
			j := putJournal()
			j.BodyFileName = "/tmp/dt-query-txn-victim"
			return j
		}},
		{name: "put jsonFileName names another query's metadata", victimInQueries: true, journal: func(_, name string) queryTxnJournal {
			j := putJournal()
			j.JSONFileName = name
			return j
		}},
		{name: "delete bodyFileName names another query's file", victimInQueries: true, journal: func(_, name string) queryTxnJournal {
			j := deleteJournal()
			j.BodyFileName = name
			return j
		}},
		{name: "put bodyFileName is not the lowercase derived name", victimInQueries: true, journal: func(_, _ string) queryTxnJournal {
			j := putJournal()
			j.BodyFileName = "x.query.DTQL"
			return j
		}},
		{name: "put hash is not a SHA-256 digest", journal: func(_, _ string) queryTxnJournal {
			j := putJournal()
			j.BodyHash = "not-a-hash"
			return j
		}},
		{name: "put prevBodyFileName without hadPrevious", journal: func(_, _ string) queryTxnJournal {
			j := putJournal()
			j.PrevBodyFileName = "x.query.sql"
			return j
		}},
		{name: "delete journal carrying put fields", journal: func(_, _ string) queryTxnJournal {
			j := deleteJournal()
			j.JSONHash = hashBytes(payload)
			return j
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Plant with a placeholder journal first, to learn the paths.
			p := plantTxn(t, putJournal(), payload, payload)
			var victim, victimName string
			if tc.victimInQueries {
				victimName = "other.query.json"
				victim = filepath.Join(p.queriesDir, victimName)
			} else {
				victim = filepath.Join(p.projectDir, "..", filepath.Base(p.projectDir)+"-victim", "dot_zshrc")
				if err := os.MkdirAll(filepath.Dir(victim), 0o755); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(victim)) })
			}
			if err := os.WriteFile(victim, []byte("ORIGINAL"), 0o644); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			j := tc.journal(relFrom(t, p.queriesDir, victim), victimName)
			b, err := json.Marshal(j)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			writeFile0600(t, filepath.Join(p.txnDir, queryTxnJournalFile), b)
			before := snapshotTxnArtifacts(t, p.txnDir)
			queriesBefore := snapshotDir(t, p.queriesDir)

			store := newFsQueriesStore(p.projectDir)
			ctx := context.Background()
			_, loadErr := store.LoadQueries(ctx, "")
			_, loadOneErr := store.LoadQuery(ctx, "whatever")
			putQ := dtqlQuery("n", "", "N")
			_, putErr := store.PutQuery(ctx, &putQ, datatug.QueryWriteCondition{IfNoneMatch: true})

			for what, err := range map[string]error{"LoadQueries": loadErr, "LoadQuery": loadOneErr, "PutQuery": putErr} {
				if err == nil {
					t.Errorf("%s: expected recovery to refuse the journal", what)
				} else if !strings.Contains(err.Error(), "not trustworthy") {
					t.Errorf("%s: expected a journal-trust refusal, got: %v", what, err)
				}
			}
			if got, err := os.ReadFile(victim); err != nil || string(got) != "ORIGINAL" {
				t.Errorf("expected the victim to be untouched, got %q (err %v)", got, err)
			}
			if _, err := os.Lstat("/tmp/dt-query-txn-victim"); err == nil {
				t.Errorf("expected nothing to be written at an absolute journal path")
			}
			assertSameSnapshot(t, "transaction directory", before, snapshotTxnArtifacts(t, p.txnDir))
			assertSameSnapshot(t, "queries root", queriesBefore, snapshotDir(t, p.queriesDir))
		})
	}
}

func TestRecovery_RefusesJournalFolderThroughASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows CI")
	}
	payload := []byte("body")
	for _, tc := range []struct{ name, folderPath, realParent, link string }{
		{name: "first segment (symfolder)", folderPath: "link", link: "link"},
		{name: "inner segment", folderPath: "real/link/sub", realParent: "real", link: "real/link"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := plantTxn(t, queryTxnJournal{
				FolderPath: tc.folderPath, ID: "x", Operation: queryTxnOpPut,
				JSONFileName: "x.query.json", JSONHash: hashBytes(payload),
				BodyFileName: "x.query.dtql", BodyHash: hashBytes(payload),
			}, payload, payload)
			outside := t.TempDir()
			if tc.realParent != "" {
				if err := os.MkdirAll(filepath.Join(p.queriesDir, tc.realParent), 0o755); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if err := os.Symlink(outside, filepath.Join(p.queriesDir, filepath.FromSlash(tc.link))); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			before := snapshotTxnArtifacts(t, p.txnDir)

			_, err := newFsQueriesStore(p.projectDir).LoadQuery(context.Background(), "y")
			if err == nil {
				t.Fatal("expected recovery to refuse a journal folder that resolves through a symlink")
			}
			if entries, _ := os.ReadDir(outside); len(entries) != 0 {
				t.Errorf("expected nothing written through the symlink, found %v", entries)
			}
			assertSameSnapshot(t, "transaction directory", before, snapshotTxnArtifacts(t, p.txnDir))
		})
	}
}

func TestQueryStore_RefusesASymlinkedQueriesRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows CI")
	}
	projectDir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(projectDir, "queries")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	store := newFsQueriesStore(projectDir)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "text")
	if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); !datatug.IsInvalidQueryLocation(err) {
		t.Errorf("PutQuery: expected InvalidQueryLocationError, got %v", err)
	}
	if err := store.SaveQuery(ctx, &q); !datatug.IsInvalidQueryLocation(err) {
		t.Errorf("SaveQuery: expected InvalidQueryLocationError, got %v", err)
	}
	if _, err := store.LoadQueryRevision(ctx, "q1"); !datatug.IsInvalidQueryLocation(err) {
		t.Errorf("LoadQueryRevision: expected InvalidQueryLocationError, got %v", err)
	}
	if _, err := ensureQueryTxnDir(filepath.Join(projectDir, "queries")); err == nil {
		t.Error("ensureQueryTxnDir: expected a symlinked queries root to be refused")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("expected nothing written through the symlinked queries root, found %v", entries)
	}
}

// TestQueryStore_RefusesAQueryTypeThatCannotNameAFile covers the same class
// as B1 on the read/delete side: a query's JSON "type" is project content,
// and was joined into the body file name unchecked - so a planted type read
// a file outside the project (LoadQuery, LoadQueryRevision, LoadQueries)
// and DeleteQuery removed it.
func TestQueryStore_RefusesAQueryTypeThatCannotNameAFile(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	projectDir := filepath.Dir(queriesDir)
	victim := filepath.Join(projectDir, "victim", "secret.txt")
	if err := os.MkdirAll(filepath.Dir(victim), 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(victim, []byte("SECRET"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.MkdirAll(queriesDir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "t.query." + "/../../victim/secret.txt" resolves to <project>/victim/secret.txt.
	meta := `{"title":"t","type":"/../../victim/secret.txt"}`
	if err := os.WriteFile(filepath.Join(queriesDir, "t.query.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.Background()

	if q, err := store.LoadQuery(ctx, "t"); err == nil {
		t.Errorf("LoadQuery: expected an error, read text %q", q.Text)
	}
	if sq, err := store.LoadQueryRevision(ctx, "t"); err == nil {
		t.Errorf("LoadQueryRevision: expected an error, read text %q", sq.Query.Text)
	}
	if _, err := store.LoadQueries(ctx, ""); err == nil {
		t.Error("LoadQueries: expected an error")
	}
	if err := store.DeleteQuery(ctx, "t"); err == nil {
		t.Error("DeleteQuery: expected an error")
	}
	replacement := dtqlQuery("t", "", "new")
	if err := store.SaveQuery(ctx, &replacement); err == nil {
		t.Error("SaveQuery over the planted record: expected an error")
	}
	if got, err := os.ReadFile(victim); err != nil || string(got) != "SECRET" {
		t.Fatalf("expected the file outside the queries root to be untouched, got %q (err %v)", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(queriesDir, "t.query.json")); err != nil || string(got) != meta {
		t.Errorf("expected the planted record to be left as it was, got %q (err %v)", got, err)
	}
}

// TestQueryStore_StillReadsLegacyTypesOutsideValidate proves the type rule
// is a file-name safety rule, not QueryDef.Validate's type list: a legacy
// record whose type Validate no longer accepts still loads.
func TestQueryStore_StillReadsLegacyTypesOutsideValidate(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	if err := os.MkdirAll(queriesDir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(queriesDir, "s.query.json"), []byte(`{"title":"s","type":"StructuredSQL"}`), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(queriesDir, "s.query.structuredsql"), []byte("BODY"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.Background()
	q, err := store.LoadQuery(ctx, "s")
	if err != nil || q.Text != "BODY" {
		t.Fatalf("LoadQuery: expected BODY, got %+v (err %v)", q, err)
	}
	sq, err := store.LoadQueryRevision(ctx, "s")
	if err != nil || sq.Query.Text != "BODY" {
		t.Fatalf("LoadQueryRevision: expected BODY, got %+v (err %v)", sq, err)
	}
}

func TestQueryTxnJournal_Validate(t *testing.T) {
	h := hashBytes([]byte("x"))
	valid := []queryTxnJournal{
		{ID: "q", Operation: queryTxnOpPut, JSONFileName: "q.query.json", JSONHash: h, BodyFileName: "q.query.sql", BodyHash: h},
		{FolderPath: "a/b", ID: "q", Operation: queryTxnOpPut, JSONFileName: "q.query.json", JSONHash: h, BodyFileName: "q.query.sql", BodyHash: h, HadPrevious: true, PrevBodyFileName: "q.query.dtql"},
		{ID: "q", Operation: queryTxnOpPut, JSONFileName: "q.query.json", JSONHash: h, BodyFileName: "q.query.sql", BodyHash: h, HadPrevious: true},
		{ID: "q", Operation: queryTxnOpDelete, JSONFileName: "q.query.json", BodyFileName: "q.query.sql"},
		{ID: "q", Operation: queryTxnOpDelete, JSONFileName: "q.query.json"}, // an incomplete record has no body
	}
	for _, j := range valid {
		if err := j.validate(); err != nil {
			t.Errorf("expected %+v to be valid, got: %v", j, err)
		}
	}
	invalid := []queryTxnJournal{
		{ID: "q", Operation: "rename", JSONFileName: "q.query.json"},
		{FolderPath: "../x", ID: "q", Operation: queryTxnOpDelete, JSONFileName: "q.query.json"},
		{ID: "../q", Operation: queryTxnOpDelete, JSONFileName: "../q.query.json"},
		{ID: "q", Operation: queryTxnOpDelete, JSONFileName: "Q.query.json"},
		{ID: "q", Operation: queryTxnOpDelete, JSONFileName: "q.query.json", BodyFileName: "../q.query.sql"},
		{ID: "q", Operation: queryTxnOpDelete, JSONFileName: "q.query.json", HadPrevious: true},
		{ID: "q", Operation: queryTxnOpPut, JSONFileName: "q.query.json", JSONHash: h, BodyHash: h},                                               // no body name
		{ID: "q", Operation: queryTxnOpPut, JSONFileName: "q.query.json", JSONHash: strings.ToUpper(h), BodyFileName: "q.query.sql", BodyHash: h}, // not hashBytes' form
		{ID: "q", Operation: queryTxnOpPut, JSONFileName: "q.query.json", JSONHash: h, BodyFileName: "q.query.sql", BodyHash: h, HadPrevious: true, PrevBodyFileName: "p.query.sql"},
	}
	for _, j := range invalid {
		if err := j.validate(); err == nil {
			t.Errorf("expected %+v to be refused", j)
		}
	}
}

func TestReadJournal_RefusesAnOversizeJournal(t *testing.T) {
	p := plantTxn(t, queryTxnJournal{ID: "q", Operation: queryTxnOpDelete, JSONFileName: "q.query.json"}, nil, nil)
	big := make([]byte, maxQueryTxnJournalSize+1)
	for i := range big {
		big[i] = ' '
	}
	writeFile0600(t, filepath.Join(p.txnDir, queryTxnJournalFile), big)
	if _, _, err := readJournal(p.txnDir); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expected an oversize journal to be refused, got: %v", err)
	}
}

// withFileOwnership swaps fileOwnedByCurrentUser for the duration of a
// test, to simulate an artifact owned by another user (which an
// unprivileged test cannot create). No test in this package runs in
// parallel, so the swap cannot leak into another test.
func withFileOwnership(t *testing.T, owned func(info os.FileInfo) bool) {
	t.Helper()
	previous := fileOwnedByCurrentUser
	fileOwnedByCurrentUser = owned
	t.Cleanup(func() { fileOwnedByCurrentUser = previous })
}

func TestEnsureQueryTxnDir_RefusesADirectoryOwnedByAnotherUser(t *testing.T) {
	root := t.TempDir()
	queriesDir := filepath.Join(root, "queries")
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	if err := os.MkdirAll(txnDir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.Chmod(txnDir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	withFileOwnership(t, func(os.FileInfo) bool { return false })

	if _, err := ensureQueryTxnDir(queriesDir); err == nil || !strings.Contains(err.Error(), "owned by another user") {
		t.Fatalf("expected a foreign transaction directory to be refused, got: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Lstat(txnDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o755 {
			t.Errorf("expected a foreign directory not to be chmod-repaired, got %v", perm)
		}
	}
	if _, err := newFsQueriesStore(root).LoadQueries(context.Background(), ""); err == nil {
		t.Error("expected a read to fail closed on a foreign transaction directory")
	}
}

func TestTxnArtifacts_OwnedByAnotherUserAreRefused(t *testing.T) {
	payload := []byte("body")
	p := plantTxn(t, queryTxnJournal{
		ID: "x", Operation: queryTxnOpPut,
		JSONFileName: "x.query.json", JSONHash: hashBytes(payload),
		BodyFileName: "x.query.dtql", BodyHash: hashBytes(payload),
	}, payload, payload)
	for _, foreign := range []string{queryTxnJournalFile, queryTxnStagedBody, queryTxnStagedJSON} {
		t.Run(foreign, func(t *testing.T) {
			withFileOwnership(t, func(info os.FileInfo) bool { return info.Name() != foreign })
			before := snapshotTxnArtifacts(t, p.txnDir)
			err := completeQueryTransaction(p.queriesDir, p.txnDir)
			if err == nil || !strings.Contains(err.Error(), "owned by another user") {
				t.Fatalf("expected %s owned by another user to be refused, got: %v", foreign, err)
			}
			assertSameSnapshot(t, "transaction directory", before, snapshotTxnArtifacts(t, p.txnDir))
			if _, err := os.Lstat(filepath.Join(p.queriesDir, "x.query.json")); err == nil {
				t.Error("expected nothing installed from a foreign artifact")
			}
		})
	}
}

func TestTxnArtifacts_DirectoryIsRefusedAsAJournal(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.Mkdir(filepath.Join(txnDir, queryTxnJournalFile), 0o700); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, _, err := readJournal(txnDir); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected a directory journal to be refused, got: %v", err)
	}
}
