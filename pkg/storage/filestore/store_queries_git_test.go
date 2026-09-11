package filestore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Task 5 Epilogue B: "Git shows ordinary uncommitted query assets and no
// commit is created; another checkout can load the pair without stored
// result rows or credentials." These tests shell out to a real git binary
// against a temp project directory - the store never invokes Git itself
// (see pkg/storage/filestore's "the file store's commit hook remains a
// no-op"), so what Git reports here is purely a consequence of what files
// exist on disk, proving the store's writes look like ordinary file
// changes to a project's own version control.

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in this environment")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")
}

func TestPutQuery_LeavesOnlyOrdinaryUncommittedGitChanges(t *testing.T) {
	projectDir := t.TempDir()
	initGitRepo(t, projectDir)
	headBefore := runGit(t, projectDir, "rev-parse", "HEAD")

	store := newFsQueriesStore(projectDir)
	q := dtqlQuery("q1", "", "from:\n  name: Invoice\n")
	if _, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	headAfter := runGit(t, projectDir, "rev-parse", "HEAD")
	if headBefore != headAfter {
		t.Fatalf("expected no commit to be created by PutQuery; HEAD moved from %s to %s", headBefore, headAfter)
	}

	// --untracked-files=all lists each new file individually rather than
	// collapsing the brand-new "queries/" directory into one line.
	status := runGit(t, projectDir, "status", "--porcelain", "--untracked-files=all")
	if !strings.Contains(status, "queries/q1.query.json") {
		t.Fatalf("expected the JSON sidecar to show as an ordinary uncommitted change, got status:\n%s", status)
	}
	if !strings.Contains(status, "queries/q1.query.dtql") {
		t.Fatalf("expected the body sidecar to show as an ordinary uncommitted change, got status:\n%s", status)
	}
	// Every status line for a brand-new file must be "??" (untracked), never
	// a staged/index change - the store must not run `git add` or similar.
	for _, line := range strings.Split(status, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "??") {
			t.Errorf("expected an untracked ('??') status line, got: %q", line)
		}
	}
}

func TestDeleteQueryRevision_LeavesOnlyOrdinaryUncommittedGitChanges(t *testing.T) {
	projectDir := t.TempDir()
	initGitRepo(t, projectDir)

	store := newFsQueriesStore(projectDir)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "text")
	stored, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}
	runGit(t, projectDir, "add", "-A")
	runGit(t, projectDir, "commit", "-q", "-m", "add q1")
	headBefore := runGit(t, projectDir, "rev-parse", "HEAD")

	if err := store.DeleteQueryRevision(ctx, "q1", stored.Revision); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}

	headAfter := runGit(t, projectDir, "rev-parse", "HEAD")
	if headBefore != headAfter {
		t.Fatalf("expected no commit to be created by DeleteQueryRevision; HEAD moved from %s to %s", headBefore, headAfter)
	}
	status := runGit(t, projectDir, "status", "--porcelain")
	if !strings.Contains(status, "D  queries/q1.query.json") && !strings.Contains(status, " D queries/q1.query.json") {
		t.Fatalf("expected the JSON sidecar to show as an ordinary deletion, got status:\n%s", status)
	}
}

// TestPutQuery_AnotherCheckoutLoadsThePairWithoutCredentials simulates
// "another checkout" by opening a brand new store rooted at the same
// directory (a fresh process/clone would see the same files) and proving
// it loads the exact persisted definition with no credential material -
// this store never accepts one (QueryDefTarget.Validate), so there is none
// to find.
func TestPutQuery_AnotherCheckoutLoadsThePairWithoutCredentials(t *testing.T) {
	projectDir := t.TempDir()
	initGitRepo(t, projectDir)

	store := newFsQueriesStore(projectDir)
	q := datatug.QueryDefWithFolderPath{
		QueryDef: datatug.QueryDef{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q1", Title: "Q1"}},
			Type:        datatug.QueryTypeSQL,
			Text:        "SELECT * FROM customers WHERE id = :CustomerId",
			Targets:     []datatug.QueryDefTarget{{Driver: "postgres", Credentials: datatug.Credentials{Username: "reporting-user"}}},
		},
	}
	if _, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reopened := newFsQueriesStore(projectDir)
	loaded, err := reopened.LoadQueryRevision(context.Background(), "q1")
	if err != nil {
		t.Fatalf("unexpected error reopening: %v", err)
	}
	if loaded.Query.Text != q.Text {
		t.Fatalf("expected the body to round-trip, got %q", loaded.Query.Text)
	}
	if len(loaded.Query.Targets) != 1 || loaded.Query.Targets[0].Password != "" {
		t.Fatalf("expected no password to be present, got targets: %+v", loaded.Query.Targets)
	}

	jsonBytes := readQueryJSONFile(t, filepath.Join(projectDir, "queries", "q1.query.json"))
	if strings.Contains(jsonBytes, "password") {
		t.Fatalf("expected the persisted JSON never to mention a password field, got: %s", jsonBytes)
	}
}

func readQueryJSONFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("unexpected error reading %s: %v", path, err)
	}
	return string(data)
}
