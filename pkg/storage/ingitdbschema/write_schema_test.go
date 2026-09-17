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

func TestWriteSchema_RefusesToOverwriteDifferingFile(t *testing.T) {
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
		t.Fatal("WriteSchema should fail when an existing file differs from the embedded schema")
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
		t.Errorf("differing file was modified; got %q", string(got))
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
