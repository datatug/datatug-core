//go:build unix

package filestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Review SF-A: in a sticky folder only a file's owner (or the folder's) may
// rename over it or remove it, although access(2) reports the folder
// writable. Another user's target there is refused before the commit.
func TestStickyFolder_AnotherUsersTarget_IsRefusedBeforeTheCommit(t *testing.T) {
	skipIfRoot(t)
	ps, queriesDir, rev := seedQ(t)
	if err := os.Chmod(queriesDir, 0o777|os.ModeSticky); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(queriesDir, 0o755) })
	withFileOwnership(t, func(info os.FileInfo) bool {
		return info.Name() != "q.query.dtql" && info.Name() != "queries"
	})
	requireRefusedWrite(t, putQ("t1", "T1", "DTQL")(ps, rev), "sticky folder")
	if fileExistsAt(filepath.Join(queriesDir, reservedQueryTxnDirName, queryTxnJournalFile)) {
		t.Error("expected nothing committed")
	}
	if got, err := ps.LoadQueryRevision(context.Background(), "q"); err != nil || got.Revision != rev {
		t.Errorf("expected q unchanged at %s, got %+v, %v", rev, got, err)
	}
}
