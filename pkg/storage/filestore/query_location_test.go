package filestore

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
		"re\u200cf",      // S3: default-ignorable, invisible in any listing
		"a/re\u200cf/b",
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
	for _, id := range []string{
		"q1", "customer-invoices", "a", "query_2", "Query.With.Dots",
		// Near misses of the Windows device names that are ordinary names.
		"CONSOLE", "COM10", "LPT12", "communication", "auxiliary", "CONIN", "nullable",
		// Non-ASCII letters remain valid.
		"запрос", "クエリ", "café",
	} {
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
		// N1: control characters (C0, DEL, C1).
		"a\x01b",
		"line\nbreak",
		"tab\tid",
		"del\x7f",
		"c1\u0085", // C1 control NEXT LINE
		// N1: bidirectional-text controls (Unicode Bidi_Control).
		"a\u202eb", // RIGHT-TO-LEFT OVERRIDE
		"a\u202ab", // LEFT-TO-RIGHT EMBEDDING
		"a\u2066b", // LEFT-TO-RIGHT ISOLATE
		"a\u2069b", // POP DIRECTIONAL ISOLATE
		"a\u200fb", // RIGHT-TO-LEFT MARK
		"a\u061cb", // ARABIC LETTER MARK
		// S3: default-ignorable code points. They are invisible, so two ids
		// differing only by one look identical in a listing, a diff or a
		// review - and on HFS+ they are not two ids at all but one file.
		"rev\u200cenue", // ZERO WIDTH NON-JOINER
		"rev\u200denue", // ZERO WIDTH JOINER
		"rev\u200benue", // ZERO WIDTH SPACE
		"rev\u2060enue", // WORD JOINER
		"rev\u206aenue", // INHIBIT SYMMETRIC SWAPPING
		"rev\u206fenue", // NOMINAL DIGIT SHAPES
		"rev\ufeffenue", // ZERO WIDTH NO-BREAK SPACE (BOM)
		"rev\u00adenue", // SOFT HYPHEN
		"rev\u034fenue", // COMBINING GRAPHEME JOINER
		"chart\ufe0f",   // VARIATION SELECTOR-16
		"tag\U000e0001", // LANGUAGE TAG
		// Not valid UTF-8: not representable on APFS/NTFS, ambiguous in git.
		"bad\xffutf8",
		// N1: every Windows device-name variant, case-insensitive.
		"COM0",
		"LPT0",
		"COM¹",
		"com²",
		"LPT³",
		"CONIN$",
		"conout$",
		"CONIN$.query",
		"CON .txt", // Windows ignores trailing spaces before the extension
		"nul.anything",
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

// Review S3: an invisible character is refused on every write path, and
// reads stay lenient, so a record that already carries one is still
// readable. Refusing beats stripping: either spelling is a name someone can
// read back, and neither is the one they typed.
func TestQueryIDs_WithInvisibleCharacters_RefusedForWritesButStillReadable(t *testing.T) {
	ctx := context.Background()
	store, queriesDir := newTestQueriesStore(t)
	const alias = "rev\u200cenue"

	q := dtqlQuery(alias, "", "BODY")
	_, putErr := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	requireInvisibleRefusal(t, "PutQuery", putErr)
	requireInvisibleRefusal(t, "SaveQuery", store.SaveQuery(ctx, &q))
	_, createErr := store.CreateQuery(ctx, q)
	requireInvisibleRefusal(t, "CreateQuery", createErr)
	_, updateErr := store.UpdateQuery(ctx, q.QueryDef)
	requireInvisibleRefusal(t, "UpdateQuery", updateErr)

	inFolder := dtqlQuery("revenue", "re\u200cf", "BODY")
	_, folderErr := store.PutQuery(ctx, &inFolder, datatug.QueryWriteCondition{IfNoneMatch: true})
	requireInvisibleRefusal(t, "PutQuery into an invisible folder", folderErr)

	if entries, err := os.ReadDir(queriesDir); err == nil && len(entries) != 0 {
		t.Errorf("expected the refused writes to leave nothing behind, got %d entries", len(entries))
	}

	// A record that already carries one - written before this rule, or by
	// another tool - stays readable through the legacy read path.
	if err := os.MkdirAll(queriesDir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	meta := []byte(`{"id":"` + alias + `","title":"Legacy","type":"DTQL"}` + "\n")
	if err := os.WriteFile(filepath.Join(queriesDir, alias+".query.json"), meta, 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(queriesDir, alias+".query.dtql"), []byte("LEGACY"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := store.LoadQuery(ctx, alias)
	if err != nil {
		t.Fatalf("expected a legacy record carrying an invisible character to stay readable, got: %v", err)
	}
	if got.Text != "LEGACY" {
		t.Errorf("expected the legacy body, got %q", got.Text)
	}
}

func requireInvisibleRefusal(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected an invisible character to be refused", what)
	}
	if !strings.Contains(err.Error(), "invisible") {
		t.Errorf("%s: expected the refusal to name the invisible character, got: %v", what, err)
	}
	if !datatug.IsInvalidQueryLocation(err) {
		t.Errorf("%s: expected a typed location refusal, got %T: %v", what, err, err)
	}
}
