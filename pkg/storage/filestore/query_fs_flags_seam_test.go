package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// lockPaths makes fileFlagsIssue report a locked entry for the given paths,
// on every platform, as chattr +i or chflags uchg would on a real one.
func lockPaths(t *testing.T, paths ...string) (unlock func()) {
	t.Helper()
	locked := make(map[string]bool, len(paths))
	for _, p := range paths {
		locked[p] = true
	}
	saved := fileFlagsIssue
	fileFlagsIssue = func(filePath string, info os.FileInfo) (string, bool) {
		if locked[filePath] {
			return "is locked (simulated)", true
		}
		return saved(filePath, info)
	}
	unlock = func() { fileFlagsIssue = saved }
	t.Cleanup(unlock)
	return unlock
}

// The same refusals as TestLockedTargets_AreRefusedBeforeTheCommit and
// TestAppendOnlyFolders_AreRefusedBeforeTheCommit, which need real BSD file
// flags, driven through the fileFlagsIssue seam so every platform runs them.
func TestSimulatedLockedTargets_AreRefusedBeforeTheCommit(t *testing.T) {
	for _, tc := range []struct {
		name, target string
		write        func(fsProjectStore, datatug.QueryRevision) error
	}{
		{"locked body, put", "q.query.dtql", putQ("t1", "T1", datatug.QueryTypeDTQL)},
		{"locked metadata, put", "q.query.json", putQ("t1", "T1", datatug.QueryTypeDTQL)},
		{"locked stale body, type change", "q.query.dtql", putQ("t1", "SELECT 1", datatug.QueryTypeSQL)},
		{"locked metadata, DeleteQuery", "q.query.json", func(ps fsProjectStore, _ datatug.QueryRevision) error {
			return ps.DeleteQuery(context.Background(), "q")
		}},
		{"locked body, DeleteQueryRevision", "q.query.dtql", func(ps fsProjectStore, rev datatug.QueryRevision) error {
			return ps.DeleteQueryRevision(context.Background(), "q", rev)
		}},
		{"locked queries folder", "", putQ("t1", "T1", datatug.QueryTypeDTQL)},
		{"locked transaction directory", reservedQueryTxnDirName, putQ("t1", "T1", datatug.QueryTypeDTQL)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps, queriesDir, rev := seedQ(t)
			unlock := lockPaths(t, filepath.Join(queriesDir, tc.target))
			err := tc.write(ps, rev)
			requireRefusedWrite(t, err, "is locked (simulated)")
			if errors.As(err, new(*queryTxnIncompleteError)) {
				t.Errorf("expected a refusal before the commit, got: %v", err)
			}
			if got, err := ps.LoadQueryRevision(context.Background(), "q"); err != nil || got.Revision != rev {
				t.Errorf("expected q unchanged at %s, got %+v, %v", rev, got, err)
			}
			unlock()
			assertStoreUsableAfterRefusal(t, ps, queriesDir)
		})
	}
}
