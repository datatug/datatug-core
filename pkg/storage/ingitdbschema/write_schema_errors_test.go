package ingitdbschema

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestWriteSchema_ErrorWhenStoreRootCannotBeCreated covers WriteSchema's
// os.MkdirAll(dir, ...) failure path: dir cannot be created because one of
// its path components is an existing regular file, not a directory.
func TestWriteSchema_ErrorWhenStoreRootCannotBeCreated(t *testing.T) {
	parent := t.TempDir()
	blocker := filepath.Join(parent, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("seed blocker file: %v", err)
	}

	dir := filepath.Join(blocker, "store")
	if err := WriteSchema(dir); err == nil {
		t.Fatal("WriteSchema should fail when the store root cannot be created")
	}
}

// TestWriteSchemaFile_MissingEmbeddedSource covers writeSchemaFile's
// fs.ReadFile error path: relPath is not part of the embedded schema.
func TestWriteSchemaFile_MissingEmbeddedSource(t *testing.T) {
	sub, err := fs.Sub(schemaFS, schemaRoot)
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}

	target := filepath.Join(t.TempDir(), "out.yaml")
	if err = writeSchemaFile(sub, "does/not/exist.yaml", target); err == nil {
		t.Fatal("writeSchemaFile should fail for a relPath absent from the embedded schema")
	}
}

// TestWriteSchemaFile_StatErrorOtherThanNotExist covers writeSchemaFile's
// default branch: os.ReadFile(target) fails with an error that is not
// "file does not exist" (here, permission denied on the parent directory).
func TestWriteSchemaFile_StatErrorOtherThanNotExist(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission checks are bypassed")
	}

	sub, err := fs.Sub(schemaFS, schemaRoot)
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}

	dir := t.TempDir()
	blockedParent := filepath.Join(dir, "blocked")
	if err = os.MkdirAll(blockedParent, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(blockedParent, "root-collections.yaml")

	// Deny traversal of the parent so opening/stating target through it
	// fails with permission denied rather than "not exist".
	if err = os.Chmod(blockedParent, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer func() { _ = os.Chmod(blockedParent, 0o755) }() // allow TempDir cleanup

	if err = writeSchemaFile(sub, ".ingitdb/root-collections.yaml", target); err == nil {
		t.Fatal("writeSchemaFile should surface a non-not-exist stat error")
	}
}

// TestWriteSchemaFile_CannotCreateTargetDirectory covers
// createFileAtomically's os.MkdirAll(dir, ...) failure path: the target file
// does not exist (a genuine ErrNotExist), but its directory cannot be
// created because the parent has no write permission.
func TestWriteSchemaFile_CannotCreateTargetDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission checks are bypassed")
	}

	sub, err := fs.Sub(schemaFS, schemaRoot)
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}

	dir := t.TempDir()
	readOnlyParent := filepath.Join(dir, "readonly")
	if err = os.MkdirAll(readOnlyParent, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err = os.Chmod(readOnlyParent, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer func() { _ = os.Chmod(readOnlyParent, 0o755) }() // allow TempDir cleanup

	// The target's own directory ("newsubdir") does not exist yet, and
	// readOnlyParent has no write permission to create it.
	target := filepath.Join(readOnlyParent, "newsubdir", "root-collections.yaml")

	if err = writeSchemaFile(sub, ".ingitdb/root-collections.yaml", target); err == nil {
		t.Fatal("writeSchemaFile should fail when its target directory cannot be created")
	}
}

// TestWriteSchemaFile_CannotWriteTargetFile covers createFileAtomically's
// os.CreateTemp failure path: the target's directory already exists, but has
// no write permission, so even the temp file used for the atomic write
// cannot be created there.
func TestWriteSchemaFile_CannotWriteTargetFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission checks are bypassed")
	}

	sub, err := fs.Sub(schemaFS, schemaRoot)
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}

	readOnlyParent := t.TempDir()
	if err = os.Chmod(readOnlyParent, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer func() { _ = os.Chmod(readOnlyParent, 0o755) }() // allow TempDir cleanup

	target := filepath.Join(readOnlyParent, "root-collections.yaml")

	if err = writeSchemaFile(sub, ".ingitdb/root-collections.yaml", target); err == nil {
		t.Fatal("writeSchemaFile should fail when it cannot write the target file")
	}
}
