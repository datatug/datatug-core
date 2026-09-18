package ingitdbschema

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteSchema_RetryAfterInterruptedWriteSucceeds simulates a process
// killed between writing a temp file and renaming it into place: an orphaned
// temp file exists for a target that was never actually created. A retry (a
// fresh WriteSchema call) must complete normally rather than mistaking the
// orphan, or the target's absence, for a content conflict. WriteSchema no
// longer cleans up other writers' temp files (see createFile's doc comment:
// a cleanup pass cannot tell its own leftovers from another writer's
// in-flight temp file), so the orphan is asserted to survive untouched, and
// unrelated to the correctly written target.
func TestWriteSchema_RetryAfterInterruptedWriteSucceeds(t *testing.T) {
	dir := t.TempDir()

	target := filepath.Join(dir, filepath.FromSlash(".ingitdb/root-collections.yaml"))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	orphan := target + ".tmp-leftover-from-a-crash"
	orphanContent := []byte("partial garbage from an interrupted write")
	if err := os.WriteFile(orphan, orphanContent, 0o644); err != nil {
		t.Fatalf("seed orphan temp file: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target must start absent, stat err = %v", err)
	}

	if err := WriteSchema(dir); err != nil {
		t.Fatalf("WriteSchema should complete a retry after an interrupted write, got: %v", err)
	}

	if _, err := os.Stat(target); err != nil {
		t.Errorf("expected target to be written: %v", err)
	}
	gotOrphan, err := os.ReadFile(orphan)
	if err != nil {
		t.Fatalf("expected the orphaned temp file to survive untouched: %v", err)
	}
	if string(gotOrphan) != string(orphanContent) {
		t.Errorf("orphaned temp file content changed: got %q, want %q", gotOrphan, orphanContent)
	}
}

// TestWriteSchema_WritesFilesWithMode0644 asserts createFile sets the mode
// of a newly written file to 0644: os.CreateTemp defaults to 0600, and
// WriteSchema's output is meant to be group/world-readable like any other
// file in the store.
func TestWriteSchema_WritesFilesWithMode0644(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; file mode bits are not enforced the same way")
	}
	dir := t.TempDir()
	if err := WriteSchema(dir); err != nil {
		t.Fatalf("WriteSchema(%s): %v", dir, err)
	}

	target := filepath.Join(dir, filepath.FromSlash(".ingitdb/root-collections.yaml"))
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat %s: %v", target, err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o644); got != want {
		t.Errorf("mode = %o, want %o", got, want)
	}
}

// TestWriteSchema_TreatsCRLFAsIdenticalToLF proves the identical-content
// check is line-ending tolerant: a file already on disk with CRLF line
// endings (as a Windows checkout with core.autocrlf would produce from the
// LF-only embedded schema) must be treated as already installed, not as a
// conflict.
func TestWriteSchema_TreatsCRLFAsIdenticalToLF(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, filepath.FromSlash(".ingitdb/root-collections.yaml"))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Use the real embedded content, but with CRLF line endings, to
	// simulate a Windows checkout of the same schema.
	lfContent, readErr := schemaFS.ReadFile("files/.ingitdb/root-collections.yaml")
	if readErr != nil {
		t.Fatalf("read embedded root-collections.yaml: %v", readErr)
	}
	crlfContent := crlfify(lfContent)
	if err := os.WriteFile(target, crlfContent, 0o644); err != nil {
		t.Fatalf("seed CRLF file: %v", err)
	}

	if err := WriteSchema(dir); err != nil {
		t.Fatalf("WriteSchema should treat a CRLF copy as identical, got: %v", err)
	}

	// It must not have been rewritten to LF: WriteSchema leaves a matching
	// file untouched, whichever line ending it used.
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read back %s: %v", target, err)
	}
	if string(got) != string(crlfContent) {
		t.Errorf("CRLF file was modified; got %q, want unchanged %q", got, crlfContent)
	}
}

func crlfify(b []byte) []byte {
	out := make([]byte, 0, len(b)+16)
	for _, c := range b {
		if c == '\n' {
			out = append(out, '\r', '\n')
			continue
		}
		out = append(out, c)
	}
	return out
}
