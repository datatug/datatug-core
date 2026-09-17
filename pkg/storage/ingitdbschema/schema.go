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
// naming its path — WriteSchema never overwrites a difference silently.
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
		return fmt.Errorf("ingitdbschema: %s already exists with different content; refusing to overwrite it", target)
	case os.IsNotExist(err):
		if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("ingitdbschema: create directory for %s: %w", target, err)
		}
		if err = os.WriteFile(target, content, 0o644); err != nil {
			return fmt.Errorf("ingitdbschema: write %s: %w", target, err)
		}
		return nil
	default:
		return fmt.Errorf("ingitdbschema: stat %s: %w", target, err)
	}
}
