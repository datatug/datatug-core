// Package ingitdbschema ships DataTug's inGitDB project-store schema as
// static files and copies them into a store directory when a project store
// is initialised.
//
// Founder decision, 2026-09-17: "DataTug project ingitdb schema should be
// defined and copy-pasted. No need to create it dynamically." and "if we
// need modify ingitdb schema it belong to ingitdb module, not dalgo." This
// package therefore contains no dynamic schema construction and imports
// neither a DALgo driver nor ingitdb-go itself: it only copies bytes it
// never parses.
//
// See spec/features/dalgo-project-store, REQ:canonical-project-layout and
// REQ:extension-namespace, and the "No dynamic schema creation" bullet of
// REQ:driver-prerequisites-are-recorded.
package ingitdbschema

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
// of the schema on disk: a file whose existing content matches byte for byte
// is left untouched (idempotent), a missing file is written, and a file that
// exists with different content is left untouched and reported as an error
// naming its path — WriteSchema never overwrites a difference silently. Each
// missing file is created atomically (temp file, fsync, hard link into
// place; see createFileAtomically), so a run interrupted partway through
// never leaves a partially written file at a schema path: a retry always
// sees that file as either genuinely absent (and completes it) or fully
// written (and is idempotent), never as a false conflict.
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

// writeSchemaFile writes one embedded schema file to target, applying the
// no-silent-overwrite and idempotency rules documented on WriteSchema.
func writeSchemaFile(sub fs.FS, relPath, target string) error {
	content, err := fs.ReadFile(sub, relPath)
	if err != nil {
		return fmt.Errorf("ingitdbschema: read embedded %s: %w", relPath, err)
	}
	existing, err := os.ReadFile(target)
	switch {
	case err == nil:
		if bytes.Equal(existing, content) {
			return nil // already installed; idempotent no-op
		}
		return conflictError(target)
	case os.IsNotExist(err):
		return createFileAtomically(target, content)
	default:
		return fmt.Errorf("ingitdbschema: stat %s: %w", target, err)
	}
}

// tempFileGlobSuffix names the temp files createFileAtomically writes,
// shared between os.CreateTemp's pattern and the glob cleanupStaleTempFiles
// uses so the two agree on what counts as "one of ours".
const tempFileGlobSuffix = ".tmp-*"

// createFileAtomically creates target with content: it writes to a uniquely
// named temporary file in target's own directory, fsyncs it, and then links
// it into place. A hard link fails outright (EEXIST) if target already
// exists instead of silently replacing it the way os.Rename would, which
// closes the check-then-write race in writeSchemaFile — if some other writer
// creates target between its not-exist check and this call, the link fails
// and this function re-reads target to apply the same
// idempotent-vs-conflicting rule, rather than clobbering whatever
// concurrently landed there.
//
// A process interrupted between writing the temp file and linking it leaves
// only an orphaned temp file, never a partial target: target is either
// absent (as if nothing happened) or fully written, so a retry never mistakes
// an interrupted write for a genuine content conflict — the on-disk state
// this closes over is exactly why writeSchemaFile's differing-content error
// is safe to treat as a real conflict rather than write-in-progress noise.
// The orphan itself is inert (WriteSchema never lists a destination
// directory to decide what to write) but is best-effort cleaned up by the
// next writer that targets the same file.
//
// Residual case: this makes creating one file atomic and race-free, but does
// not make the whole multi-file WriteSchema call atomic, nor does it add
// cross-file locking — dalgo2ingitdb's own contract is single-writer (see its
// Database.NoConcurrency field doc), and this package inherits that
// assumption rather than layering a second locking scheme on top of it.
func createFileAtomically(target string, content []byte) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ingitdbschema: create directory for %s: %w", target, err)
	}
	cleanupStaleTempFiles(dir, filepath.Base(target))

	tmp, err := os.CreateTemp(dir, filepath.Base(target)+tempFileGlobSuffix)
	if err != nil {
		return fmt.Errorf("ingitdbschema: create temp file for %s: %w", target, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once linked; cleans up on any earlier-returned error

	if _, err = tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ingitdbschema: write temp file for %s: %w", target, err)
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ingitdbschema: sync temp file for %s: %w", target, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("ingitdbschema: close temp file for %s: %w", target, err)
	}

	if err = os.Link(tmpPath, target); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("ingitdbschema: create %s: %w", target, err)
		}
		// target appeared between writeSchemaFile's not-exist check and this
		// call. Re-read it and apply the same idempotent-vs-conflicting rule
		// rather than assuming either outcome.
		existing, readErr := os.ReadFile(target)
		if readErr != nil {
			return fmt.Errorf("ingitdbschema: %s appeared concurrently but could not be read: %w", target, readErr)
		}
		if bytes.Equal(existing, content) {
			return nil
		}
		return conflictError(target)
	}
	return nil
}

// cleanupStaleTempFiles best-effort removes any temp file createFileAtomically
// previously left behind for base (e.g. from a process interrupted before it
// could link its temp file into place). It never fails WriteSchema: a glob or
// remove error here is silently ignored, since an orphan left in place is
// merely inert clutter, not a correctness problem.
func cleanupStaleTempFiles(dir, base string) {
	matches, err := filepath.Glob(filepath.Join(dir, base+tempFileGlobSuffix))
	if err != nil {
		return
	}
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

// conflictError reports that target already holds content different from
// the schema this package would write, without touching it.
func conflictError(target string) error {
	return fmt.Errorf("ingitdbschema: %s already exists with different content; refusing to overwrite it", target)
}
