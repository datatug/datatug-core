// Package ingitdbschema ships DataTug's inGitDB project-store schema as
// static files and copies them into a store directory when a project store
// is initialised.
//
// Founder decision, 2026-09-17: "DataTug project ingitdb schema should be
// defined and copy-pasted. No need to create it dynamically." and "if we
// need modify ingitdb schema it belong to ingitdb module, not dalgo." This
// package therefore contains no dynamic schema construction and imports no
// DALgo driver. It imports ingitdb-go/ingitdb as a test-only dependency (to
// load and validate this package's own schema in tests); production code
// (schema.go) only copies bytes it never parses.
//
// See spec/features/dalgo-project-store, REQ:canonical-project-layout and
// REQ:extension-namespace, and the "No dynamic schema creation" bullet of
// REQ:driver-prerequisites-are-recorded.
package ingitdbschema

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// schemaFS embeds every file under files/, including the dotted directories
// inGitDB's on-disk format requires (.ingitdb/, .collection/). The "all:"
// prefix is required for go:embed to include names starting with "." or "_";
// without it those directories would be silently dropped.
//
//go:embed all:files
var schemaFS embed.FS

// schemaRoot is the name of the embedded directory that mirrors a store
// root: schemaFS's paths are all rooted at "files/...".
const schemaRoot = "files"

// WriteSchema copies DataTug's static inGitDB schema into dir, which becomes
// (or already is) an inGitDB store root: dir/.ingitdb/root-collections.yaml
// and dir/ext/.collection/... after this call.
//
// WriteSchema is safe to call against a store that already has some or all
// of the schema on disk. A file whose existing content matches the embedded
// schema (ignoring CRLF-vs-LF line-ending differences, so a Windows checkout
// with autocrlf is not reported as a conflict) is left untouched (idempotent).
// Beyond that, the two files this package ships are treated differently
// (founder decision, 2026-09-17):
//
//   - Every file under ext/ is DataTug-owned: WriteSchema writes it whether
//     it was previously absent or held different (older) content, upgrading
//     it in place. This is the "overwrite our own files" rule — ext/ holds
//     nothing this package did not itself put there.
//   - .ingitdb/root-collections.yaml is NOT DataTug-owned: it may be shared
//     with other extensions' own root-collection entries, so a differing
//     copy is left untouched and reported as a conflict error naming its
//     path, exactly as before this decision. WriteSchema does not attempt to
//     merge or selectively update entries in it.
//
// Every file WriteSchema does write (whether previously absent or being
// upgraded) is created via a temp file in its own directory, fsynced and
// renamed into place (see createFile), so a run interrupted partway through
// never leaves a partially written file at a schema path: a retry always
// sees that file as either its old content (untouched) or the new content
// (fully written), never partial.
//
// WriteSchema assumes a single writer, matching dalgo2ingitdb's own contract
// (its Database.NoConcurrency field doc): it does not detect or arbitrate a
// concurrent WriteSchema call against the same directory.
func WriteSchema(dir string) error {
	sub, err := fs.Sub(schemaFS, schemaRoot)
	if err != nil {
		return fmt.Errorf("ingitdbschema: embedded schema is broken: %w", err)
	}
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ingitdbschema: create store root %s: %w", dir, err)
	}
	return fs.WalkDir(sub, ".", func(relPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if relPath == "." {
			return nil
		}
		target := filepath.Join(dir, filepath.FromSlash(relPath))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return writeSchemaFile(sub, relPath, target)
	})
}

// dataTugOwnedPathPrefix is the subtree WriteSchema always writes,
// upgrading a differing existing file rather than refusing it. relPath
// values come from fs.WalkDir over the embedded schemaFS, which — like
// every io/fs path — always uses "/" regardless of host OS.
const dataTugOwnedPathPrefix = "ext/"

// isDataTugOwnedPath reports whether relPath (embedded-schema-relative,
// slash-separated) is a file this package exclusively owns, as opposed to
// .ingitdb/root-collections.yaml, which may be shared with other
// extensions' own entries.
func isDataTugOwnedPath(relPath string) bool {
	return relPath == "ext" || strings.HasPrefix(relPath, dataTugOwnedPathPrefix)
}

// writeSchemaFile writes one embedded schema file to target, applying the
// idempotency and ownership rules documented on WriteSchema.
func writeSchemaFile(sub fs.FS, relPath, target string) error {
	content, err := fs.ReadFile(sub, relPath)
	if err != nil {
		return fmt.Errorf("ingitdbschema: read embedded %s: %w", relPath, err)
	}
	existing, err := os.ReadFile(target)
	switch {
	case err == nil:
		if contentsEqualIgnoringLineEndings(existing, content) {
			return nil // already installed; idempotent no-op
		}
		if isDataTugOwnedPath(relPath) {
			return createFile(target, content) // DataTug-owned: upgrade in place
		}
		return conflictError(target) // shared file: never overwritten silently
	case os.IsNotExist(err):
		return createFile(target, content)
	default:
		return fmt.Errorf("ingitdbschema: read %s: %w", target, err)
	}
}

// contentsEqualIgnoringLineEndings reports whether a and b are equal once
// every CRLF pair in each is normalized to a bare LF. A checkout with
// Windows-style line endings (e.g. Git's core.autocrlf) must not be reported
// as a conflict against the embedded schema, which is authored with LF only.
func contentsEqualIgnoringLineEndings(a, b []byte) bool {
	return bytes.Equal(normalizeLineEndings(a), normalizeLineEndings(b))
}

func normalizeLineEndings(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

// createFile creates target with content: it writes to a uniquely named
// temporary file in target's own directory, sets its mode to 0644 (a plain
// os.CreateTemp file is 0600), fsyncs it, and renames it into place.
// os.Rename is atomic on the file systems this driver targets, so a process
// interrupted between writing the temp file and the rename leaves only an
// orphaned temp file — target itself is either absent (as if nothing
// happened) or fully written, never partial, so a retry never mistakes an
// interrupted write for a genuine content conflict.
//
// This assumes a single writer (see WriteSchema's doc): os.Rename replaces
// an existing target rather than failing, so a concurrent writer racing this
// call could have its own write silently overwritten. That is an accepted
// consequence of the single-writer contract, not a gap this function closes.
//
// An orphaned temp file from an earlier interrupted run is left in place:
// createFile does not scan target's directory for other temp files to clean
// up, because doing so cannot distinguish its own leftovers from another
// concurrent writer's in-progress temp file (removing someone else's
// in-flight temp file is worse than leaving an inert one behind).
func createFile(target string, content []byte) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ingitdbschema: create directory for %s: %w", target, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(target)+".tmp-*")
	if err != nil {
		return fmt.Errorf("ingitdbschema: create temp file for %s: %w", target, err)
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err = tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ingitdbschema: write temp file for %s: %w", target, err)
	}
	if err = tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ingitdbschema: chmod temp file for %s: %w", target, err)
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ingitdbschema: sync temp file for %s: %w", target, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("ingitdbschema: close temp file for %s: %w", target, err)
	}
	if err = os.Rename(tmpPath, target); err != nil {
		return fmt.Errorf("ingitdbschema: create %s: %w", target, err)
	}
	renamed = true
	return nil
}

// conflictError reports that target already holds content different from
// the schema this package would write, without touching it.
func conflictError(target string) error {
	return fmt.Errorf("ingitdbschema: %s already exists with different content; refusing to overwrite it", target)
}
