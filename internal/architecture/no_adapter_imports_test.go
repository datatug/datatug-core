package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// adapterImportPrefix is an import path opening a quoted string that names a separate DALgo
// adapter module (dal-go/dalgo2<engine>). Built by concatenation so this file does not match
// itself. Adapters bundled in the DALgo core module, such as dal-go/dalgo/adapters/dalgo2memory
// and .../dalgo2fs, do not start with it and stay allowed (AGENTS.md).
const adapterImportPrefix = `"github.com/dal-go/` + `dalgo2`

// adapterImportLines returns the trimmed lines of src that import a separate DALgo adapter module.
// It is a plain text search, so single, grouped and aliased imports all match.
func adapterImportLines(src string) []string {
	var lines []string
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, adapterImportPrefix) {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return lines
}

func TestNoDALgoAdapterModuleImports(t *testing.T) {
	scanned := 0
	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		rel, _ := filepath.Rel(moduleRoot, path)
		for _, line := range adapterImportLines(string(content)) {
			t.Errorf("%s: %s imports a separate DALgo adapter module: datatug-core may import the DALgo core module only (AGENTS.md)", rel, line)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk module: %v", err)
	}
	if scanned == 0 {
		t.Fatal("scanned no .go files: the walk is broken, so the check would pass vacuously")
	}
}

func TestAdapterImportCheckCatchesKnownViolations(t *testing.T) {
	// Positive control, so a broken prefix cannot make the scan pass vacuously.
	planted := "package p\n\nimport (\n\t\"fmt\"\n\tfs " + adapterImportPrefix + "firestore\"\n)\n\nimport " + adapterImportPrefix + "postgres\"\n"
	if got := adapterImportLines(planted); len(got) != 2 {
		t.Errorf("planted adapter imports: found %v, want 2 lines", got)
	}
	for _, allowed := range []string{
		`import "github.com/dal-go/dalgo/adapters/dalgo2memory"`,
		`import "github.com/dal-go/dalgo/adapters/dalgo2fs"`,
		`import "github.com/dal-go/dalgo/dal"`,
	} {
		if got := adapterImportLines(allowed); len(got) != 0 {
			t.Errorf("core-module import %s wrongly flagged: %v", allowed, got)
		}
	}
}
