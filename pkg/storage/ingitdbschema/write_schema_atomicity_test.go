package ingitdbschema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteSchema_RetryAfterInterruptedWriteSucceeds simulates a process
// killed between writing a temp file and linking it into place: an orphaned
// temp file exists for a target that was never actually created. A retry
// (a fresh WriteSchema call) must complete normally rather than mistaking the
// orphan, or the target's absence, for a content conflict — and must clean
// the orphan up rather than leaving it behind forever.
func TestWriteSchema_RetryAfterInterruptedWriteSucceeds(t *testing.T) {
	dir := t.TempDir()

	target := filepath.Join(dir, filepath.FromSlash(".ingitdb/root-collections.yaml"))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	orphan := target + ".tmp-leftover-from-a-crash"
	if err := os.WriteFile(orphan, []byte("partial garbage from an interrupted write"), 0o644); err != nil {
		t.Fatalf("seed orphan temp file: %v", err)
	}

	if err := WriteSchema(dir); err != nil {
		t.Fatalf("WriteSchema should complete a retry after an interrupted write, got: %v", err)
	}

	if _, err := os.Stat(target); err != nil {
		t.Errorf("expected target to be written: %v", err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("expected orphaned temp file to be cleaned up, stat err = %v", err)
	}
}

// TestCreateFileAtomically_ConcurrentIdenticalContent covers the EEXIST
// fallback in createFileAtomically for the case where another writer created
// target (with matching content) between writeSchemaFile's not-exist check
// and this call: the link fails with EEXIST, and re-reading target finds it
// already matches, so the call must succeed rather than error.
func TestCreateFileAtomically_ConcurrentIdenticalContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "root-collections.yaml")
	content := []byte("ext: ext\n")
	if err := os.WriteFile(target, content, 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	if err := createFileAtomically(target, content); err != nil {
		t.Fatalf("createFileAtomically should be idempotent when target already matches, got: %v", err)
	}
}

// TestCreateFileAtomically_ConcurrentDifferingContent covers the EEXIST
// fallback's conflicting branch: target exists (created concurrently, from
// this function's point of view) with content that differs from what it was
// asked to write.
func TestCreateFileAtomically_ConcurrentDifferingContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "root-collections.yaml")
	if err := os.WriteFile(target, []byte("ext: somewhere-else\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	err := createFileAtomically(target, []byte("ext: ext\n"))
	if err == nil {
		t.Fatal("createFileAtomically should fail when target concurrently gained differing content")
	}
	if !strings.Contains(err.Error(), target) {
		t.Errorf("error should name %s, got: %v", target, err)
	}
}

// TestCreateFileAtomically_ConcurrentTargetUnreadable covers the EEXIST
// fallback's own read failure: target exists but cannot be read back (here,
// because it is a directory), so createFileAtomically cannot even decide
// whether it matches and must report a distinct error rather than silently
// treating it as either outcome.
func TestCreateFileAtomically_ConcurrentTargetUnreadable(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "root-collections.yaml")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("seed target directory: %v", err)
	}

	err := createFileAtomically(target, []byte("ext: ext\n"))
	if err == nil {
		t.Fatal("createFileAtomically should fail when target exists but cannot be read")
	}
	if !strings.Contains(err.Error(), "appeared concurrently") {
		t.Errorf("error should explain the concurrent-appearance case, got: %v", err)
	}
}

// TestCleanupStaleTempFiles_RemovesOrphan is a focused unit test of the
// cleanup helper itself: it must remove a temp file matching base's pattern
// and leave unrelated files untouched.
func TestCleanupStaleTempFiles_RemovesOrphan(t *testing.T) {
	dir := t.TempDir()
	orphan := filepath.Join(dir, "root-collections.yaml.tmp-abc123")
	unrelated := filepath.Join(dir, "root-collections.yaml")
	if err := os.WriteFile(orphan, []byte("stale"), 0o644); err != nil {
		t.Fatalf("seed orphan: %v", err)
	}
	if err := os.WriteFile(unrelated, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("seed unrelated file: %v", err)
	}

	cleanupStaleTempFiles(dir, "root-collections.yaml")

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("expected orphan to be removed, stat err = %v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("expected unrelated file to survive: %v", err)
	}
}
