package filestore

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Review B1: a stuck transaction may only be scoped to the query it names
// when the store can prove, from disk, which files that transaction owns.
// Before the fix, attribution fell back to a path key, so renaming the
// query's folder made the stuck transaction stop covering it and a plain
// read served the half-installed mix - exactly the state the AC forbids.
// Now anything short of proof covers everything and fails closed.
//
// Every case below wedges a write after its commit point (the body is
// installed, the JSON metadata rename fails), so the pair on disk really is
// the mix "t0"/"T1", then moves the folder out from under the journal and
// reads through a freshly opened store, which re-runs recovery.

// seedStuckMix creates a project holding the unrelated query "other" at the
// root plus "<folder>/<id>" at title "t0"/body "T0", then wedges a write of
// that query so the body installs as "T1" and the metadata stays "t0". The
// returned store is left stuck; the rename keeps failing until the test
// ends.
func seedStuckMix(t *testing.T, folder, id string) (fsProjectStore, string) {
	t.Helper()
	ctx := context.Background()
	ps, queriesDir := newPreflightProject(t)
	seed := dtqlQuery(id, folder, "T0")
	seed.Title = "t0"
	if _, err := ps.PutQuery(ctx, &seed, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error seeding %s/%s: %v", folder, id, err)
	}
	failQueryTargetStep(t, "rename", id+".query.json")
	update := dtqlQuery(id, folder, "T1")
	update.Title = "t1"
	requireStuck(t, "the wedging write", ps.SaveQuery(ctx, &update))
	requireMixOnDisk(t, filepath.Join(queriesDir, filepath.FromSlash(folder)), id)
	return ps, queriesDir
}

// requireMixOnDisk proves the premise of every case here: the pair really
// is half installed, so a read that served it would serve a mix.
func requireMixOnDisk(t *testing.T, dir, id string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, id+".query.dtql"))
	if err != nil || string(body) != "T1" {
		t.Fatalf("expected the new body installed, got %q (err %v)", body, err)
	}
	meta, err := os.ReadFile(filepath.Join(dir, id+".query.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := string(meta); !strings.Contains(got, `"title": "t0"`) && !strings.Contains(got, `"title":"t0"`) {
		t.Fatalf("expected the old metadata still in place, got %s", got)
	}
}

// requireNoMixServed is the AC clause itself: whatever else happens, a read
// must never return the old metadata joined to the new body.
func requireNoMixServed(t *testing.T, store fsProjectStore, fullID string) {
	t.Helper()
	got, err := store.LoadQueryRevision(context.Background(), fullID)
	if err != nil {
		return // refused, which is always an acceptable answer here
	}
	if got.Query.Title == "t0" && got.Query.Text == "T1" {
		t.Fatalf("%s: served the half-installed mix title=%q text=%q", fullID, got.Query.Title, got.Query.Text)
	}
}

// TestStuckWrite_WhenItsFolderMovesOutFromUnderIt_RefusesEverything covers
// the reproductions the review named: the folder renamed away, replaced by
// a symlink, replaced by a different directory, and deleted. In each the
// store can no longer prove which files the transaction owns, so every
// query is refused and nothing serves the mix.
func TestStuckWrite_WhenItsFolderMovesOutFromUnderIt_RefusesEverything(t *testing.T) {
	for _, tc := range []struct {
		name     string
		symlinks bool
		// disturb moves queries/a out of the way; it returns the folder
		// the half-installed pair now lives in, or "" when it is gone.
		disturb func(t *testing.T, queriesDir string) string
	}{
		{
			name: "renamed away",
			disturb: func(t *testing.T, queriesDir string) string {
				rename(t, filepath.Join(queriesDir, "a"), filepath.Join(queriesDir, "b"))
				return "b"
			},
		},
		{
			name:     "replaced by a symlink to itself",
			symlinks: true,
			disturb: func(t *testing.T, queriesDir string) string {
				rename(t, filepath.Join(queriesDir, "a"), filepath.Join(queriesDir, "moved"))
				if err := os.Symlink(filepath.Join(queriesDir, "moved"), filepath.Join(queriesDir, "a")); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return "moved"
			},
		},
		{
			name: "replaced by a different, empty directory",
			disturb: func(t *testing.T, queriesDir string) string {
				rename(t, filepath.Join(queriesDir, "a"), filepath.Join(queriesDir, "moved"))
				if err := os.Mkdir(filepath.Join(queriesDir, "a"), 0o755); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return "moved"
			},
		},
		{
			name: "deleted",
			disturb: func(t *testing.T, queriesDir string) string {
				if err := os.RemoveAll(filepath.Join(queriesDir, "a")); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return ""
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.symlinks {
				skipSymlinksOnWindows(t)
			}
			ctx := context.Background()
			_, queriesDir := seedStuckMix(t, "a", "revenue")
			where := tc.disturb(t, queriesDir)

			fresh := newFsProjectStore("p", filepath.Dir(queriesDir))
			if where != "" {
				requireNoMixServed(t, fresh, where+"/revenue")
				_, err := fresh.LoadQueryRevision(ctx, where+"/revenue")
				requireStuck(t, "LoadQueryRevision("+where+"/revenue)", err)
				_, err = fresh.LoadQueries(ctx, where)
				requireStuck(t, "LoadQueries("+where+")", err)
			}
			// The store cannot prove what the transaction owns, so it fails
			// closed for every query, exactly as it did before slots existed.
			_, err := fresh.LoadQuery(ctx, "other")
			requireStuck(t, "LoadQuery(other), an unrelated query", err)
			third := dtqlQuery("third", "", "THIRD")
			_, err = fresh.PutQuery(ctx, &third, datatug.QueryWriteCondition{IfNoneMatch: true})
			requireStuck(t, "PutQuery(third), an unrelated write", err)
		})
	}
}

func rename(t *testing.T, from, to string) {
	t.Helper()
	if err := os.Rename(from, to); err != nil {
		t.Fatalf("unexpected error renaming %s: %v", from, err)
	}
}

// A folder spelled another way a file system may resolve to the same
// directory still reaches the stuck query, so the scoped case keeps
// covering every alias of its own folder.
func TestStuckWrite_CoversItsFolderUnderAnAliasSpelling(t *testing.T) {
	for _, tc := range []struct{ name, folder, alias string }{
		{"case variant", "a", "A"},
		{"composed and decomposed", "caf\u00e9", "cafe\u0301"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			ps, _ := seedStuckMix(t, tc.folder, "revenue")
			requireNoMixServed(t, ps, tc.alias+"/revenue")
			_, err := ps.LoadQueryRevision(ctx, tc.alias+"/revenue")
			requireStuck(t, "LoadQueryRevision through "+tc.alias, err)
			_, err = ps.LoadQueries(ctx, tc.alias)
			requireStuck(t, "LoadQueries("+tc.alias+")", err)
		})
	}
}

// The name key must never be finer than the file system underneath it. On
// HFS+ "rev<U+200C>enue" is the very same file as "revenue" - measured on
// a scratch HFS+ volume, along with U+FEFF and U+206A - so a stuck write of
// "revenue" has to cover those spellings too, or one of them would read
// straight through to the half-installed pair. The legacy read path is what
// can still address such a name: the revisioned path refuses an invisible
// character in an id outright.
func TestStuckWrite_CoversAnIDSpelledWithAnInvisibleCharacter(t *testing.T) {
	ctx := context.Background()
	ps, _ := seedStuckMix(t, "", "revenue")
	for _, alias := range []string{
		"revenue",
		"REVENUE",
		"rev\u200cenue", // zero-width non-joiner, merged by HFS+
		"rev\ufeffenue", // byte-order mark, merged by HFS+
		"rev\u206aenue", // inhibit symmetric swapping, merged by HFS+
		"rev\u200benue", // zero-width space
		"rev\u00adenue", // soft hyphen
		"rev\u034fenue", // combining grapheme joiner
	} {
		_, err := ps.LoadQuery(ctx, alias)
		requireStuck(t, "LoadQuery("+strconv.Quote(alias)+")", err)
	}
}

// A committed transaction that has installed nothing cannot leave a mix
// anywhere, so the store proves that instead of the folder and keeps
// working - the common crash-before-install case. The query it names is
// still refused, so a competing write cannot race the committed journal.
func TestStuckWrite_ProvedToHaveInstalledNothing_StaysScoped(t *testing.T) {
	ctx := context.Background()
	_, queriesDir := newPreflightProject(t)
	txnDir := filepath.Join(queriesDir, reservedQueryTxnDirName)
	n := dtqlQuery("revenue", "", "NEW")
	n.Title = "t0"
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
		ID: "revenue", Operation: queryTxnOpPut,
		JSONFileName: "revenue.query.json", JSONHash: hashBytes(jsonBytes),
		BodyFileName: "revenue.query.dtql", BodyHash: hashBytes([]byte(n.Text)),
	}
	if err := commitQueryTransaction(queriesDir, txnDir, j); err != nil {
		t.Fatalf("unexpected error committing: %v", err)
	}
	// The first install step fails, so nothing of the pair is on disk.
	failQueryTargetStep(t, "rename", "revenue.query.dtql")

	fresh := newFsProjectStore("p", filepath.Dir(queriesDir))
	if q, err := fresh.LoadQuery(ctx, "other"); err != nil || q.Text != "OTHER" {
		t.Errorf("expected an unrelated query to keep working, got: %v", err)
	}
	_, err = fresh.LoadQueryRevision(ctx, "revenue")
	requireStuck(t, "LoadQueryRevision(revenue)", err)
	// Nothing was installed, so no spelling can reach a half-installed
	// pair - there is none to reach.
	for _, alias := range []string{"revenue", "REVENUE", "rev\u200cenue"} {
		requireNoMixServed(t, fresh, alias)
	}
}
