package filestore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func TestVisibilityOpenedRootPreservesArchiveAfterPathReplacement(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "project")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeVisibilityFixture(t, dir, visibilityFile, `{"version":1,"archived":[{"kind":"server","id":"postgres/warehouse"},{"kind":"meaning","id":"project/person"}]}`)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	if err := os.Rename(dir, filepath.Join(parent, "original")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	archived, err := ReadProjectArchivedItemsFromRoot(root)
	if err != nil || len(archived) != 2 || !archived[datatug.ProjectObjectRef{Kind: "server", ID: "postgres/warehouse"}] || !archived[datatug.ProjectObjectRef{Kind: "meaning", ID: "project/person"}] {
		t.Fatalf("archive came from replacement pathname: %+v %v", archived, err)
	}
	if _, err := root.Stat(visibilityFile); err != nil {
		t.Fatal("reader closed caller-owned root", err)
	}
}

func TestVisibilityOpenedRootAndLegacyReaderRejectMalformedDocuments(t *testing.T) {
	for _, body := range []string{"", "null", " \n null \t", "[]", `{"version":2}`, `{"version":"1"}`, `{"version":1,"version":1}`, `{"version":1,"archived":[{"kind":"invalid","id":"q"}]}`, `{"version":1,"archived":[{"kind":"query","id":"q"},{"kind":"query","id":"q"}]}`} {
		t.Run(body, func(t *testing.T) {
			dir := t.TempDir()
			writeVisibilityFixture(t, dir, visibilityFile, body)
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = root.Close() }()
			if rows, err := ReadProjectArchivedItemsFromRoot(root); err == nil || rows != nil {
				t.Fatal("malformed anchored archive became all active", rows, err)
			}
			if rows, err := ReadProjectArchivedItems(dir); err == nil || rows != nil {
				t.Fatal("malformed legacy archive became all active", rows, err)
			}
		})
	}
}

func TestVisibilityOpenedRootMissingAndUnsafeFiles(t *testing.T) {
	for _, mode := range []string{"missing", "directory", "symlink", "oversize", "closed-root"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = root.Close() }()
			switch mode {
			case "directory":
				if err := os.Mkdir(filepath.Join(dir, visibilityFile), 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				writeVisibilityFixture(t, dir, "other.json", `{"version":1}`)
				if err := os.Symlink("other.json", filepath.Join(dir, visibilityFile)); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				writeVisibilityFixture(t, dir, visibilityFile, strings.Repeat(" ", (2<<20)+1))
			case "closed-root":
				if err := root.Close(); err != nil {
					t.Fatal(err)
				}
			}
			rows, err := ReadProjectArchivedItemsFromRoot(root)
			if mode == "missing" {
				if err != nil || rows == nil || len(rows) != 0 {
					t.Fatal("missing archive is a valid empty snapshot", rows, err)
				}
			} else if err == nil || rows != nil {
				t.Fatal("unsafe archive became all active", rows, err)
			}
		})
	}
	if rows, err := ReadProjectArchivedItemsFromRoot(nil); err == nil || rows != nil {
		t.Fatal("nil root accepted")
	}
}

type visibilityRootFault struct {
	*os.Root
	open func(string, int, fs.FileMode) (*os.File, error)
}

func (root visibilityRootFault) OpenFile(name string, flags int, mode fs.FileMode) (*os.File, error) {
	return root.open(name, flags, mode)
}

func TestVisibilityOpenedRootRefusesOpenRaceAndReadFailures(t *testing.T) {
	for _, mode := range []string{"open-error", "stat-error", "different-file", "directory-after-lstat", "read-error", "growth", "symlink-after-lstat"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			writeVisibilityFixture(t, dir, visibilityFile, `{"version":1}`)
			writeVisibilityFixture(t, dir, "other.json", `{"version":1}`)
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = root.Close() }()
			fault := visibilityRootFault{Root: root, open: func(name string, flags int, perm fs.FileMode) (*os.File, error) {
				if name != visibilityFile || flags != os.O_RDONLY|openNoFollowFlags || perm != 0 {
					t.Fatalf("unsafe open scope/flags: %q %d %d", name, flags, perm)
				}
				switch mode {
				case "open-error":
					return nil, errors.New("open failed")
				case "stat-error":
					file, err := root.OpenFile(name, flags, perm)
					if err != nil {
						return nil, err
					}
					if err := file.Close(); err != nil {
						t.Fatal(err)
					}
					return file, nil
				case "different-file":
					return root.OpenFile("other.json", flags, perm)
				case "directory-after-lstat":
					return root.OpenFile(".", flags, perm)
				case "read-error":
					return root.OpenFile(name, os.O_WRONLY, perm)
				case "growth":
					writeVisibilityFixture(t, dir, visibilityFile, `{"version":1}`+strings.Repeat(" ", 2<<20))
				case "symlink-after-lstat":
					if err := os.Remove(filepath.Join(dir, visibilityFile)); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink("other.json", filepath.Join(dir, visibilityFile)); err != nil {
						t.Fatal(err)
					}
				}
				return root.OpenFile(name, flags, perm)
			}}
			state, err := readProjectVisibilityRoot(fault)
			rows, err := projectArchivedItems(state, err)
			if err == nil || rows != nil {
				t.Fatal("raced/unreadable archive became all active", rows, err)
			}
		})
	}
}
