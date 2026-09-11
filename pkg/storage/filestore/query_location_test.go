package filestore

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Task 2: enforce safe query locations. These prove every refusal the plan
// calls out - absolute paths, ".."/"." traversal, embedded separators/NUL,
// the reserved transaction namespace - fails validation before any I/O,
// and that a symlinked folder is rejected once resolved against a real
// queries root.

func TestValidateQueryFolderPath_Valid(t *testing.T) {
	for _, fp := range []string{"", "folder1", "folder1/sub2", "a/b/c"} {
		if err := validateQueryFolderPath(fp); err != nil {
			t.Errorf("expected folder path %q to be valid, got: %v", fp, err)
		}
	}
}

func TestValidateQueryFolderPath_Invalid(t *testing.T) {
	cases := []string{
		"/etc",              // absolute
		"folder1/",          // trailing slash -> empty interior segment
		"/",                 // absolute root
		"folder1//sub2",     // empty interior segment
		"..",                // traversal
		"../escape",         // traversal
		"folder1/..",        // traversal
		"folder1/../../etc", // traversal
		".",                 // current-dir segment
		"folder1\\sub2",     // backslash
		"folder1\x00",       // NUL
		".dt-query-txn",     // reserved transaction namespace
		"folder1/.dt-query-txn",
		".hidden", // leading dot reserved (defense in depth)
		"C:\\Windows",
		"folder1:stream", // Windows-illegal char
	}
	for _, fp := range cases {
		err := validateQueryFolderPath(fp)
		if err == nil {
			t.Errorf("expected folder path %q to be rejected", fp)
			continue
		}
		if !datatug.IsInvalidQueryLocation(err) {
			t.Errorf("expected folder path %q to fail with InvalidQueryLocationError, got %T: %v", fp, err, err)
		}
	}
}

func TestValidateQueryID_Valid(t *testing.T) {
	for _, id := range []string{"q1", "customer-invoices", "a", "query_2", "Query.With.Dots"} {
		if err := validateQueryID(id); err != nil {
			t.Errorf("expected id %q to be valid, got: %v", id, err)
		}
	}
}

func TestValidateQueryID_Invalid(t *testing.T) {
	cases := []string{
		"",
		".",
		"..",
		"a/b",
		"a\\b",
		"a\x00b",
		".hidden",
		".dt-query-txn",
		"/etc/passwd",
		"con", // Windows reserved device name (case-insensitive)
		"COM1",
		"trailing.", // Windows: trailing dot
		"trailing ", // Windows: trailing space
		"a:b",       // Windows-illegal char
		"a*b",
		"a?b",
		"a<b>",
		"a|b",
		`a"b`,
	}
	for _, id := range cases {
		err := validateQueryID(id)
		if err == nil {
			t.Errorf("expected id %q to be rejected", id)
			continue
		}
		if !datatug.IsInvalidQueryLocation(err) {
			t.Errorf("expected id %q to fail with InvalidQueryLocationError, got %T: %v", id, err, err)
		}
	}
}

func TestResolveQueryLocation_ContainedUnderRoot(t *testing.T) {
	root := t.TempDir()
	store := newFsQueriesStore(root)
	queriesRoot := filepath.Join(root, "queries")

	dir, err := store.resolveQueryLocation("folder1/sub2", "q1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(queriesRoot, "folder1", "sub2")
	if dir != expected {
		t.Fatalf("expected resolved dir %q, got %q", expected, dir)
	}
}

func TestResolveQueryLocation_RejectsInvalidInputBeforeIO(t *testing.T) {
	root := t.TempDir()
	store := newFsQueriesStore(root)

	_, err := store.resolveQueryLocation("../escape", "q1")
	if !datatug.IsInvalidQueryLocation(err) {
		t.Fatalf("expected InvalidQueryLocationError, got %T: %v", err, err)
	}
	// Nothing should have been created outside (or inside) the project.
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatalf("unexpected error reading root: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files created for a rejected request, found: %v", entries)
	}
}

func TestResolveQueryLocation_RejectsSymlinkedFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows CI")
	}
	root := t.TempDir()
	queriesRoot := filepath.Join(root, "queries")
	if err := os.MkdirAll(queriesRoot, 0o777); err != nil {
		t.Fatalf("failed to create queries root: %v", err)
	}
	outsideTarget := t.TempDir()
	if err := os.Symlink(outsideTarget, filepath.Join(queriesRoot, "folder1")); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	store := newFsQueriesStore(root)
	_, err := store.resolveQueryLocation("folder1", "q1")
	if !datatug.IsInvalidQueryLocation(err) {
		t.Fatalf("expected InvalidQueryLocationError for a symlinked folder, got %T: %v", err, err)
	}

	// Nothing should have been written into the symlink target.
	entries, readErr := os.ReadDir(outsideTarget)
	if readErr != nil {
		t.Fatalf("unexpected error reading symlink target: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files written through the symlink, found: %v", entries)
	}
}

func TestResolveQueryLocation_RejectsNonDirectoryParent(t *testing.T) {
	root := t.TempDir()
	queriesRoot := filepath.Join(root, "queries")
	if err := os.MkdirAll(queriesRoot, 0o777); err != nil {
		t.Fatalf("failed to create queries root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(queriesRoot, "folder1"), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	store := newFsQueriesStore(root)
	_, err := store.resolveQueryLocation("folder1", "q1")
	if !datatug.IsInvalidQueryLocation(err) {
		t.Fatalf("expected InvalidQueryLocationError for a non-directory parent, got %T: %v", err, err)
	}
}
