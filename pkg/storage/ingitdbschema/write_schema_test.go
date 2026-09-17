package ingitdbschema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSchema_FreshDirectory(t *testing.T) {
	dir := t.TempDir()

	if err := WriteSchema(dir); err != nil {
		t.Fatalf("WriteSchema(%s): %v", dir, err)
	}

	for _, rel := range []string{
		".ingitdb/root-collections.yaml",
		"ext/.collection/definition.yaml",
		"ext/.collection/subcollections/projects/definition.yaml",
		"ext/.collection/subcollections/projects/subcollections/queries/definition.yaml",
		"ext/.collection/subcollections/projects/subcollections/environments/subcollections/servers/definition.yaml",
		"ext/.collection/subcollections/projects/subcollections/dbdrivers/subcollections/dbservers/definition.yaml",
	} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected file %s to exist: %v", p, err)
		}
	}
}

func TestWriteSchema_IdempotentWhenIdentical(t *testing.T) {
	dir := t.TempDir()

	if err := WriteSchema(dir); err != nil {
		t.Fatalf("first WriteSchema(%s): %v", dir, err)
	}
	// Second call over identical content must succeed without error.
	if err := WriteSchema(dir); err != nil {
		t.Fatalf("second WriteSchema(%s) should be idempotent, got: %v", dir, err)
	}
}

// TestWriteSchema_RefusesToOverwriteDifferingSharedFile covers
// .ingitdb/root-collections.yaml specifically: it is NOT DataTug-owned (it
// may carry other extensions' own root-collection entries), so a differing
// copy is left untouched and reported as a conflict — unlike a file under
// ext/, which WriteSchema upgrades in place (see the next test).
func TestWriteSchema_RefusesToOverwriteDifferingSharedFile(t *testing.T) {
	dir := t.TempDir()

	target := filepath.Join(dir, filepath.FromSlash(".ingitdb/root-collections.yaml"))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("ext: somewhere-else\n"), 0o644); err != nil {
		t.Fatalf("seed differing file: %v", err)
	}

	err := WriteSchema(dir)
	if err == nil {
		t.Fatal("WriteSchema should fail when the shared root-collections.yaml differs from the embedded schema")
	}
	if !strings.Contains(err.Error(), target) {
		t.Errorf("error should name the conflicting path %s, got: %v", target, err)
	}

	// The pre-existing (differing) content must be left untouched.
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read back %s: %v", target, readErr)
	}
	if string(got) != "ext: somewhere-else\n" {
		t.Errorf("differing shared file was modified; got %q", string(got))
	}
}

// TestWriteSchema_UpgradesADifferingDataTugOwnedFile covers the opposite
// rule for ext/: it is exclusively DataTug-owned, so an older copy left over
// from a previous version of this package is replaced with the embedded
// (current) content rather than reported as a conflict.
func TestWriteSchema_UpgradesADifferingDataTugOwnedFile(t *testing.T) {
	dir := t.TempDir()

	target := filepath.Join(dir, filepath.FromSlash("ext/.collection/definition.yaml"))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oldContent := []byte("# an older version of this file\nrecord_file:\n    name: 'old'\n")
	if err := os.WriteFile(target, oldContent, 0o644); err != nil {
		t.Fatalf("seed older file: %v", err)
	}

	if err := WriteSchema(dir); err != nil {
		t.Fatalf("WriteSchema should upgrade an older DataTug-owned file, got: %v", err)
	}

	want, err := schemaFS.ReadFile("files/ext/.collection/definition.yaml")
	if err != nil {
		t.Fatalf("read embedded ext/.collection/definition.yaml: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read back %s: %v", target, err)
	}
	if string(got) != string(want) {
		t.Errorf("older ext/ file was not upgraded; got %q, want %q", got, want)
	}
}

func TestWriteSchema_CreatesStoreRootWhenMissing(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "does", "not", "exist", "yet")

	if err := WriteSchema(dir); err != nil {
		t.Fatalf("WriteSchema(%s): %v", dir, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ingitdb", "root-collections.yaml")); err != nil {
		t.Errorf("expected schema written under newly created root: %v", err)
	}
}

func TestWriteSchema_PartialExistingContentIsCompletedNotDuplicated(t *testing.T) {
	dir := t.TempDir()

	// Pre-write exactly one file identical to the embedded content; the rest
	// of the tree does not exist yet. WriteSchema must fill in the rest and
	// leave the pre-written file alone.
	content, err := schemaFS.ReadFile(rootCollectionsEmbeddedPath)
	if err != nil {
		t.Fatalf("read embedded root-collections.yaml: %v", err)
	}
	target := filepath.Join(dir, filepath.FromSlash(".ingitdb/root-collections.yaml"))
	if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err = os.WriteFile(target, content, 0o644); err != nil {
		t.Fatalf("seed identical file: %v", err)
	}

	if err = WriteSchema(dir); err != nil {
		t.Fatalf("WriteSchema(%s): %v", dir, err)
	}
	if _, err = os.Stat(filepath.Join(dir, "ext", ".collection", "definition.yaml")); err != nil {
		t.Errorf("expected the rest of the schema to be written: %v", err)
	}
}

const rootCollectionsEmbeddedPath = "files/.ingitdb/root-collections.yaml"
