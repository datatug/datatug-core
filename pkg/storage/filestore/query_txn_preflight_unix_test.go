//go:build unix

package filestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Review X1, the triggers that need unix file types or permissions: a
// dangling symlink, a FIFO or an unreadable file at the new body name, a
// symlink that arrives in a git clone, and a folder that does not accept
// new files. See query_txn_preflight_test.go for the others and for what
// assertStoreUsableAfterRefusal checks.

func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission checks")
	}
}

func TestPutQuery_RefusesUnixEntriesAtTheNewBodyNameAndStaysUsable(t *testing.T) {
	for _, tc := range []struct {
		name  string
		plant func(t *testing.T, target string)
		want  string
	}{
		{"dangling symlink", func(t *testing.T, target string) {
			if err := os.Symlink("/nonexistent/target", target); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}, "symlink"},
		{"FIFO", func(t *testing.T, target string) { mkfifo(t, target) }, "not a regular file"},
		{"unreadable file", func(t *testing.T, target string) {
			skipIfRoot(t)
			if err := os.WriteFile(target, []byte("x"), 0); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}, "permission denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps, queriesDir := newPreflightProject(t)
			tc.plant(t, filepath.Join(queriesDir, "new.query.dtql"))
			q := dtqlQuery("new", "", "NEW")
			err := within(t, 5*time.Second, func() error {
				_, err := ps.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true})
				return err
			})
			requireRefusedWrite(t, err, tc.want)
			if tc.name != "unreadable file" { // a permission failure is not a location refusal
				requireTypedQueryLocation(t, tc.name, err, "new.query.dtql")
			}
			assertStoreUsableAfterRefusal(t, ps, queriesDir)
		})
	}
}

// The review's reproduction 1: a repository carrying a symlink at a query's
// body name is cloned, and that query is then created.
func TestPutQuery_RefusesASymlinkFromAGitCloneAndStaysUsable(t *testing.T) {
	parent := t.TempDir()
	origin := filepath.Join(parent, "origin")
	if err := os.MkdirAll(filepath.Join(origin, "queries"), 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	initGitRepo(t, origin)
	writePreflightProjectFile(t, origin)
	for name, content := range map[string]string{
		"queries/other.query.json": `{"title":"o","type":"DTQL"}`,
		"queries/other.query.dtql": "OTHER",
	} {
		if err := os.WriteFile(filepath.Join(origin, name), []byte(content), 0o644); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if err := os.Symlink("/nonexistent/elsewhere", filepath.Join(origin, "queries", "report.query.dtql")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	runGit(t, origin, "add", "-A")
	runGit(t, origin, "commit", "-q", "-m", "seed")
	runGit(t, parent, "clone", "-q", "origin", "clone")
	clone := filepath.Join(parent, "clone")
	queriesDir := filepath.Join(clone, "queries")
	if info, err := os.Lstat(filepath.Join(queriesDir, "report.query.dtql")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Skipf("git did not check the symlink out as a symlink here (%v)", err)
	}

	ps := newFsProjectStore("p", clone)
	ctx := context.Background()
	if _, err := ps.LoadQueries(ctx, ""); err != nil {
		t.Fatalf("unexpected error reading the fresh clone: %v", err)
	}
	report := dtqlQuery("report", "", "REPORT")
	_, err := ps.PutQuery(ctx, &report, datatug.QueryWriteCondition{IfNoneMatch: true})
	requireRefusedWrite(t, err, "symlink")
	assertStoreUsableAfterRefusal(t, ps, queriesDir)
}

// A folder made read-only lets staging and the journal succeed but not the
// install's rename (or a delete's removal), which would leave a committed
// journal recovery retries forever. commitQueryTransaction checks the
// folder accepts new files before committing.
func TestQueryWrites_RefuseAFolderThatDoesNotAcceptNewFilesAndStayUsable(t *testing.T) {
	skipIfRoot(t)
	ps, queriesDir := newPreflightProject(t)
	ctx := context.Background()
	inRO := dtqlQuery("inro", "ro", "IN RO")
	if _, err := ps.PutQuery(ctx, &inRO, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}
	roDir := filepath.Join(queriesDir, "ro")
	if err := os.Chmod(roDir, 0o555); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	second := dtqlQuery("second", "ro", "SECOND")
	_, err := ps.PutQuery(ctx, &second, datatug.QueryWriteCondition{IfNoneMatch: true})
	requireRefusedWrite(t, err, "does not accept new files")
	assertStoreUsableAfterRefusal(t, ps, queriesDir)

	requireRefusedWrite(t, ps.DeleteQuery(ctx, "ro/inro"), "does not accept new files")
	assertStoreUsableAfterRefusal(t, ps, queriesDir)

	got, err := ps.LoadQuery(ctx, "ro/inro")
	if err != nil || got.Text != "IN RO" {
		t.Errorf("expected ro/inro unchanged, got %+v (err %v)", got, err)
	}
}
