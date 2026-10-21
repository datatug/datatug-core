package filestore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// saveSmallProject saves a whole small project - one of every kind of file
// the store writes - into root.
func saveSmallProject(t *testing.T, root string) {
	t.Helper()
	ctx := context.Background()
	project := plainWriteProject()
	project.Entities = datatug.Entities{plainWriteEntity()}
	project.Environments = datatug.Environments{plainWriteEnvironment()}
	project.DbModels = datatug.DbModels{plainWriteDbModel()}
	project.Boards = datatug.Boards{plainWriteBoard()}
	project.Queries = &datatug.QueriesFolder{}
	if err := newFsProjectStore("p1", root).SaveProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	for _, k := range writeKinds() {
		if err := k.write(root); err != nil {
			t.Fatalf("%s: %v", k.name, err)
		}
	}
}

// TestSaveOfASmallProjectWritesTheSameTreeAsBefore pins the bytes, folders
// and file names a save of a whole small project produces. The hash was taken
// from the file store at the commit before its writes went through plain
// files and plain folders only (the same test run on that commit gives the
// same hash), so a change in what a clean project receives fails here.
func TestSaveOfASmallProjectWritesTheSameTreeAsBefore(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	saveSmallProject(t, root)
	// Creation metadata is intentionally new; keep the historic byte snapshot
	// for all other metadata, paths and sidecars.
	for _, name := range []string{"queries/q1.query.json", "queries/sub/q1.query.json"} {
		p := filepath.Join(root, name)
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(raw), "\n")
		filtered := lines[:0]
		for _, line := range lines {
			if !strings.Contains(line, `"createdAt":`) {
				filtered = append(filtered, line)
			}
		}
		if err = os.WriteFile(p, []byte(strings.Join(filtered, "\n")), 0600); err != nil {
			t.Fatal(err)
		}
	}
	const want = "79614d1b9964f1097678c4e7752d755ca0dd5694831ee5e58fc04995b9d4418b"
	if got := treeHash(t, root); got != want {
		t.Fatalf("tree hash = %s, want %s", got, want)
	}
	if _, err := os.Lstat(filepath.Join(root, "datatug-project.json")); err != nil {
		t.Fatal(err)
	}
}
