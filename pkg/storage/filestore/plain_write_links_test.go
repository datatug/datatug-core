package filestore

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// The file store writes only plain files in plain folders of the project: it
// does not write through a link. For each kind of file it writes (linkSweepKinds),
// these tests put a link that leads out of the project in the file's place and
// in each folder above it, and require the write to be refused, the outside
// files byte-identical and nothing new outside. Where a platform cannot make a
// link (skipSymlinksOnWindows), internal/plainfs covers the refusal with a
// fake Lstat.

// linkFixture is a project folder next to a folder outside it.
type linkFixture struct {
	root    string // the project folder
	outside string // a folder outside it, with a file in it
}

func newLinkFixture(t *testing.T) linkFixture {
	t.Helper()
	skipSymlinksOnWindows(t)
	base := t.TempDir()
	f := linkFixture{root: filepath.Join(base, "proj"), outside: filepath.Join(base, "outside")}
	for _, dir := range []string{f.root, f.outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(f.outside, "keep.txt"), []byte("outside content"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// mkdirsAbove makes the folders above rel inside the project (not rel itself).
func (f linkFixture) mkdirsAbove(t *testing.T, rel string) {
	t.Helper()
	if dir := path.Dir(rel); dir != "." {
		if err := os.MkdirAll(filepath.Join(f.root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// mirror makes, outside, the folders and a file at the same relative path as
// the write's file, with content a followed link would overwrite.
func (f linkFixture) mirror(t *testing.T, outsideDir, file string) {
	t.Helper()
	target := filepath.Join(outsideDir, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("outside content"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f linkFixture) link(t *testing.T, target, rel string) {
	t.Helper()
	if err := os.Symlink(target, filepath.Join(f.root, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
}

// requireRefusedAndOutsideIntact checks the write error and that nothing
// outside changed, and that the error does not say where a link leads.
func (f linkFixture) requireRefusedAndOutsideIntact(t *testing.T, err error, before string, leavesAlone bool) {
	t.Helper()
	switch {
	case err != nil && strings.Contains(err.Error(), f.outside):
		t.Errorf("the error names where a link leads: %v", err)
	case err == nil && !leavesAlone:
		t.Error("the write through a link was not refused")
	}
	if after := treeHash(t, f.outside); after != before {
		t.Error("something outside the project changed")
	}
}

func TestWritesRefuseALinkInTheFilesPlace(t *testing.T) {
	for _, k := range linkSweepKinds() {
		for _, dangling := range []bool{false, true} {
			name := k.name + " live link"
			if dangling {
				name = k.name + " dangling link"
			}
			t.Run(name, func(t *testing.T) {
				f := newLinkFixture(t)
				f.mkdirsAbove(t, k.file)
				target := filepath.Join(f.outside, "keep.txt")
				if dangling {
					target = filepath.Join(f.outside, "not-there.json")
				}
				f.link(t, target, k.file)
				before := treeHash(t, f.outside)
				err := k.write(f.root)
				f.requireRefusedAndOutsideIntact(t, err, before, k.leavesAlone)
				if info, statErr := os.Lstat(filepath.Join(f.root, filepath.FromSlash(k.file))); statErr != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Errorf("the link in the file's place was replaced: %v", statErr)
				}
			})
		}
	}
}

func TestWritesRefuseALinkInAFolderAboveTheFile(t *testing.T) {
	for _, k := range linkSweepKinds() {
		folders := append([]string{}, k.extraFolders...)
		for dir := path.Dir(k.file); dir != "."; dir = path.Dir(dir) {
			folders = append(folders, dir)
		}
		for _, folder := range folders {
			t.Run(k.name+" via "+folder, func(t *testing.T) {
				f := newLinkFixture(t)
				f.mkdirsAbove(t, folder)
				// What a followed link would write over: the same path outside.
				outsideDir := filepath.Join(f.outside, "dir")
				f.mirror(t, outsideDir, strings.TrimPrefix(k.file, folder+"/"))
				f.link(t, outsideDir, folder)
				before := treeHash(t, f.outside)
				err := k.write(f.root)
				f.requireRefusedAndOutsideIntact(t, err, before, false)
				if info, statErr := os.Lstat(filepath.Join(f.root, filepath.FromSlash(folder))); statErr != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Errorf("the link in the folder was replaced: %v", statErr)
				}
			})
		}
	}
}

func TestDeletesRefuseALinkInAFolderAboveTheFile(t *testing.T) {
	for _, k := range linkSweepKinds() {
		if k.remove == nil {
			continue
		}
		file := k.file
		if k.removeFile != "" {
			file = k.removeFile
		}
		for dir := path.Dir(file); dir != "."; dir = path.Dir(dir) {
			t.Run(k.name+" via "+dir, func(t *testing.T) {
				f := newLinkFixture(t)
				f.mkdirsAbove(t, dir)
				outsideDir := filepath.Join(f.outside, "dir")
				f.mirror(t, outsideDir, strings.TrimPrefix(file, dir+"/"))
				f.link(t, outsideDir, dir)
				before := treeHash(t, f.outside)
				err := k.remove(f.root)
				f.requireRefusedAndOutsideIntact(t, err, before, false)
			})
		}
	}
}

func TestDeletesRefuseALinkInTheFilesPlace(t *testing.T) {
	for _, k := range linkSweepKinds() {
		if k.remove == nil {
			continue
		}
		file := k.file
		if k.removeFile != "" {
			file = k.removeFile
		}
		t.Run(k.name, func(t *testing.T) {
			f := newLinkFixture(t)
			f.mkdirsAbove(t, file)
			f.link(t, filepath.Join(f.outside, "keep.txt"), file)
			before := treeHash(t, f.outside)
			err := k.remove(f.root)
			f.requireRefusedAndOutsideIntact(t, err, before, k.removesTree)
		})
	}
}

// A folder item is saved at a folder path given by the caller; one that leaves
// the project is refused whether or not anything is a link.
func TestSaveFolderRefusesAPathThatLeavesTheProject(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	err := newFsFoldersStore(root).SaveFolder(context.Background(), "../../outside", plainWriteFolder())
	if err == nil {
		t.Fatal("expected a path outside the project to be refused")
	}
	if _, statErr := os.Lstat(filepath.Join(filepath.Dir(root), "outside")); statErr == nil {
		t.Fatal("a folder was made outside the project")
	}
}

// A save to a project folder that is itself reached through a link is the
// caller's own path and works: the project folder is trusted as given, only
// what is below it is not.
func TestSaveThroughALinkedProjectFolderStillWorks(t *testing.T) {
	skipSymlinksOnWindows(t)
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := writeKinds()[1].write(link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(real, "datatug-project.json")); err != nil {
		t.Fatal(err)
	}
}
