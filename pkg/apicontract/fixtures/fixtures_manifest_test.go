package fixtures

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// brokenOpenFS fails every Open call - exercises both fs.ReadDir's and
// fs.ReadFile's error path, since both fall back to Open when the concrete
// fs.FS (like embed.FS in the read-directory-listing-fails case) does not
// implement the ReadDirFS/ReadFileFS shortcut interfaces.
type brokenOpenFS struct{}

func (brokenOpenFS) Open(name string) (fs.File, error) {
	return nil, errors.New("boom: cannot open " + name)
}

// brokenReadFS lists one file successfully but fails to open it - exercises
// manifestFrom's ReadFile error path specifically, once ReadDir has already
// succeeded.
type brokenReadFS struct{}

func (brokenReadFS) Open(name string) (fs.File, error) {
	if name == "." {
		return nil, errors.New("brokenReadFS.Open(\".\") is not implemented directly")
	}
	return nil, errors.New("boom: cannot open " + name)
}

func (brokenReadFS) ReadDir(string) ([]fs.DirEntry, error) {
	return []fs.DirEntry{fakeDirEntry{name: "broken.json"}}, nil
}

// dirSkippingFS lists one subdirectory and one file, proving manifestFrom
// skips directory entries rather than trying to read them as fixtures.
type dirSkippingFS struct{}

func (dirSkippingFS) Open(name string) (fs.File, error) {
	return nil, errors.New("dirSkippingFS.Open not implemented: " + name)
}

func (dirSkippingFS) ReadDir(string) ([]fs.DirEntry, error) {
	return []fs.DirEntry{fakeDirEntry{name: "subdir", dir: true}}, nil
}

type fakeDirEntry struct {
	name string
	dir  bool
}

func (e fakeDirEntry) Name() string { return e.name }
func (e fakeDirEntry) IsDir() bool  { return e.dir }
func (e fakeDirEntry) Type() fs.FileMode {
	if e.dir {
		return fs.ModeDir
	}
	return 0
}
func (e fakeDirEntry) Info() (fs.FileInfo, error) { return fakeFileInfo{e.name}, nil }

type fakeFileInfo struct{ name string }

func (i fakeFileInfo) Name() string       { return i.name }
func (i fakeFileInfo) Size() int64        { return 0 }
func (i fakeFileInfo) Mode() fs.FileMode  { return 0 }
func (i fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (i fakeFileInfo) IsDir() bool        { return false }
func (i fakeFileInfo) Sys() any           { return nil }

func TestManifestFrom_ReadDirError(t *testing.T) {
	if _, err := manifestFrom(brokenOpenFS{}); err == nil {
		t.Fatal("expected an error when the directory listing itself fails")
	}
}

func TestManifestFrom_ReadFileError(t *testing.T) {
	if _, err := manifestFrom(brokenReadFS{}); err == nil {
		t.Fatal("expected an error when reading one listed file fails")
	}
}

func TestManifestFrom_SkipsDirectoryEntries(t *testing.T) {
	entries, err := manifestFrom(dirSkippingFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected the directory entry to be skipped, got %d entries", len(entries))
	}
}

func TestManifest_MatchesDirectoryContent(t *testing.T) {
	entries, err := Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one fixture entry")
	}

	dirEntries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var wantCount int
	for _, de := range dirEntries {
		if !de.IsDir() && filepath.Ext(de.Name()) == ".json" {
			wantCount++
		}
	}
	if len(entries) != wantCount {
		t.Fatalf("Manifest() returned %d entries, directory has %d .json files", len(entries), wantCount)
	}
}

func TestManifest_SortedByName(t *testing.T) {
	entries, err := Manifest()
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(entries); i++ {
		if entries[i-1].Name >= entries[i].Name {
			t.Fatalf("not sorted: %q >= %q", entries[i-1].Name, entries[i].Name)
		}
	}
}

func TestManifest_SHA256MatchesFileContent(t *testing.T) {
	entries, err := Manifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := Read(e.Name)
		if err != nil {
			t.Fatalf("%s: %v", e.Name, err)
		}
		sum := sha256.Sum256(data)
		want := hex.EncodeToString(sum[:])
		if e.SHA256 != want {
			t.Errorf("%s: SHA256 = %s, want %s", e.Name, e.SHA256, want)
		}
	}
}

func TestRead_UnknownFile(t *testing.T) {
	if _, err := Read("does-not-exist.json"); err == nil {
		t.Fatal("expected an error for a nonexistent fixture")
	}
}
