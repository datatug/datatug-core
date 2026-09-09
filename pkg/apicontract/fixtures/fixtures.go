// Package fixtures holds the frozen JSON fixtures for pkg/apicontract's
// normative transport types (spec/features/core-investigation-loop/
// api-contract.md, hub datatug/datatug) - one file per envelope/error case
// listed in the appendix's "Acceptance and migration" section. Server
// (datatug-cli) and client (datatug-apps) consume these same files, pinned
// by the SHA-256 digests Manifest returns, so drift between the two is
// caught rather than silently diverging.
package fixtures

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed *.json
var files embed.FS

// Entry is one frozen fixture file's name and content hash.
type Entry struct {
	Name   string
	SHA256 string
}

// Manifest returns every fixture file's name and SHA-256 hex digest, sorted
// by name, so a consumer can pin an exact set of frozen fixtures and detect
// drift from the pinned datatug-core version.
func Manifest() ([]Entry, error) {
	return manifestFrom(files)
}

// manifestFrom does Manifest's work over any fs.FS, so a broken filesystem
// can exercise its error paths directly rather than leaving them untested
// dead code (the real embed.FS above never actually fails at runtime).
func manifestFrom(fsys fs.FS) ([]Entry, error) {
	dirEntries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("apicontract/fixtures: %w", err)
	}
	entries := make([]Entry, 0, len(dirEntries))
	for _, de := range dirEntries {
		if de.IsDir() {
			continue
		}
		data, err := fs.ReadFile(fsys, de.Name())
		if err != nil {
			return nil, fmt.Errorf("apicontract/fixtures: %s: %w", de.Name(), err)
		}
		sum := sha256.Sum256(data)
		entries = append(entries, Entry{Name: de.Name(), SHA256: hex.EncodeToString(sum[:])})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// Read returns one fixture file's raw bytes by name.
func Read(name string) ([]byte, error) {
	data, err := files.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("apicontract/fixtures: %s: %w", name, err)
	}
	return data, nil
}
