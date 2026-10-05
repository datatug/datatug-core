package plainfs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// symlinkOrSkip makes link point at target, or skips the test where the
// platform or the user cannot make symbolic links (Windows without the
// privilege). The refusal itself is then covered by the fake-Lstat tests,
// which need no link.
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot make a symbolic link here (%v); the refusal is covered by the fake Lstat tests", err)
	}
}

func mustWrite(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readString(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(name string) bool {
	_, err := os.Lstat(name)
	return err == nil
}

// requireRefused asserts err is a refusal that names rel and does not leak
// anything from the outside folder.
func requireRefused(t *testing.T, err error, rel, outside string) {
	t.Helper()
	if !errors.Is(err, ErrNotPlain) {
		t.Fatalf("want ErrNotPlain, got %v", err)
	}
	if !strings.Contains(err.Error(), rel) {
		t.Fatalf("error %q does not name %q", err, rel)
	}
	if outside != "" && strings.Contains(err.Error(), outside) {
		t.Fatalf("error %q names where a link leads (%s)", err, outside)
	}
}

type fakeInfo struct{ mode fs.FileMode }

func (f fakeInfo) Name() string       { return "fake" }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeInfo) Sys() any           { return nil }

// withLstat makes every Lstat answer with the given function for the test.
func withLstat(t *testing.T, fn func(string) (fs.FileInfo, error)) {
	t.Helper()
	old := lstat
	lstat = fn
	t.Cleanup(func() { lstat = old })
}

func TestMkdirAll(t *testing.T) {
	t.Run("creates the folders and is repeatable", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "proj")
		dir := filepath.Join(root, "a", "b", "c")
		for i := 0; i < 2; i++ {
			if err := MkdirAll(root, dir); err != nil {
				t.Fatal(err)
			}
		}
		if info, err := os.Lstat(dir); err != nil || !info.IsDir() {
			t.Fatalf("folder missing: %v", err)
		}
	})
	t.Run("the project folder itself", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "proj")
		if err := MkdirAll(root, root); err != nil {
			t.Fatal(err)
		}
		if !exists(root) {
			t.Fatal("project folder not created")
		}
	})
	t.Run("a link in a folder above is refused and nothing is made outside", func(t *testing.T) {
		base := t.TempDir()
		root, outside := filepath.Join(base, "proj"), filepath.Join(base, "outside")
		if err := os.MkdirAll(filepath.Join(root, "x"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(outside, 0o755); err != nil {
			t.Fatal(err)
		}
		symlinkOrSkip(t, outside, filepath.Join(root, "x", "link"))
		err := MkdirAll(root, filepath.Join(root, "x", "link", "deeper"))
		requireRefused(t, err, "x/link", outside)
		entries, _ := os.ReadDir(outside)
		if len(entries) != 0 {
			t.Fatalf("something was made outside: %v", entries)
		}
	})
	t.Run("a file where a folder is needed is refused", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "f"), "x")
		requireRefused(t, MkdirAll(root, filepath.Join(root, "f", "sub")), "f", "")
	})
	t.Run("outside the project is refused", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "proj")
		requireRefused(t, MkdirAll(root, filepath.Join(root, "..", "elsewhere")), "elsewhere", "")
	})
	t.Run("a relative path against an absolute root is refused", func(t *testing.T) {
		requireRefused(t, MkdirAll(t.TempDir(), "relative/path"), "relative/path", "")
	})
	t.Run("a failing root create is reported", func(t *testing.T) {
		old := mkdirAll
		mkdirAll = func(string, fs.FileMode) error {
			return &fs.PathError{Op: "mkdir", Path: "/abs/proj", Err: syscall.EACCES}
		}
		t.Cleanup(func() { mkdirAll = old })
		err := MkdirAll("/abs/proj", "/abs/proj/a")
		if !errors.Is(err, syscall.EACCES) || strings.Contains(err.Error(), "/abs/proj") {
			t.Fatalf("got %v", err)
		}
	})
}

func TestFakeLstat(t *testing.T) {
	root := t.TempDir()
	proj := filepath.ToSlash(root)
	t.Run("link folder", func(t *testing.T) {
		withLstat(t, func(string) (fs.FileInfo, error) { return fakeInfo{fs.ModeSymlink | fs.ModeDir}, nil })
		requireRefused(t, walkRoot(root, "a/b"), "a", "")
	})
	t.Run("junction-like folder", func(t *testing.T) {
		withLstat(t, func(string) (fs.FileInfo, error) { return fakeInfo{fs.ModeIrregular}, nil })
		requireRefused(t, walkRoot(root, "a"), "a", "")
	})
	t.Run("link file", func(t *testing.T) {
		withLstat(t, func(p string) (fs.FileInfo, error) {
			if filepath.ToSlash(p) == proj+"/f.json" {
				return fakeInfo{fs.ModeSymlink}, nil
			}
			return fakeInfo{fs.ModeDir}, nil
		})
		_, err := CreateFile(root, filepath.Join(root, "f.json"))
		requireRefused(t, err, "f.json", "")
		requireRefused(t, Remove(root, filepath.Join(root, "f.json")), "f.json", "")
	})
	t.Run("folder in a file place", func(t *testing.T) {
		withLstat(t, func(p string) (fs.FileInfo, error) { return fakeInfo{fs.ModeDir}, nil })
		_, err := CreateFile(root, filepath.Join(root, "f.json"))
		requireRefused(t, err, "f.json", "")
		requireRefused(t, Remove(root, filepath.Join(root, "f.json")), "f.json", "")
	})
	t.Run("lstat failing", func(t *testing.T) {
		withLstat(t, func(string) (fs.FileInfo, error) {
			return nil, &fs.PathError{Op: "lstat", Path: "/proj/a", Err: syscall.EACCES}
		})
		err := walkRoot(root, "a")
		if !errors.Is(err, syscall.EACCES) || strings.Contains(err.Error(), "/proj") {
			t.Fatalf("got %v", err)
		}
	})
}

// walkRoot is the read-only walk, for tests.
func walkRoot(root, rel string) error {
	_, err := walkFolders(root, rel, false)
	return err
}

func TestWalkFoldersFailures(t *testing.T) {
	t.Run("mkdir failing", func(t *testing.T) {
		root := t.TempDir()
		old := mkdir
		mkdir = func(string, fs.FileMode) error { return errors.New("disk full") }
		t.Cleanup(func() { mkdir = old })
		_, err := walkFolders(root, "a", true)
		if err == nil || !strings.Contains(err.Error(), "a: disk full") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a folder made by someone else meanwhile is checked like any other", func(t *testing.T) {
		root := t.TempDir()
		old := mkdir
		mkdir = func(p string, perm fs.FileMode) error {
			_ = os.Mkdir(p, perm)
			return fs.ErrExist
		}
		t.Cleanup(func() { mkdir = old })
		if _, err := walkFolders(root, "a", true); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a link made by someone else meanwhile is refused", func(t *testing.T) {
		base := t.TempDir()
		root, outside := filepath.Join(base, "p"), filepath.Join(base, "o")
		_ = os.Mkdir(root, 0o755)
		_ = os.Mkdir(outside, 0o755)
		old := mkdir
		mkdir = func(p string, perm fs.FileMode) error {
			if err := os.Symlink(outside, p); err != nil {
				return err
			}
			return fs.ErrExist
		}
		t.Cleanup(func() { mkdir = old })
		_, err := walkFolders(root, "a", true)
		if err != nil && !errors.Is(err, ErrNotPlain) {
			t.Skipf("cannot make a symbolic link here (%v)", err)
		}
		requireRefused(t, err, "a", outside)
	})
	t.Run("lstat failing after the folder was made", func(t *testing.T) {
		root := t.TempDir()
		calls := 0
		withLstat(t, func(p string) (fs.FileInfo, error) {
			calls++
			if calls == 1 {
				return nil, fs.ErrNotExist
			}
			return nil, &fs.PathError{Op: "lstat", Path: p, Err: syscall.EIO}
		})
		_, err := walkFolders(root, "a", true)
		if !errors.Is(err, syscall.EIO) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a non-path error is wrapped as it is", func(t *testing.T) {
		withLstat(t, func(string) (fs.FileInfo, error) { return nil, errors.New("odd") })
		err := walkRoot("/proj", "a")
		if err == nil || err.Error() != "a: odd" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCreateFile(t *testing.T) {
	t.Run("writes a new file, making the folders", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "proj")
		name := filepath.Join(root, "a", "b.json")
		f, err := CreateFile(root, name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(f, "hello")
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		if got := readString(t, name); got != "hello" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("truncates a plain file", func(t *testing.T) {
		root := t.TempDir()
		name := filepath.Join(root, "f")
		mustWrite(t, name, "a long old content")
		f, err := CreateFile(root, name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(f, "new")
		_ = f.Close()
		if got := readString(t, name); got != "new" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("a link in the file's place is refused, live or dangling", func(t *testing.T) {
		for _, dangling := range []bool{false, true} {
			base := t.TempDir()
			root, outside := filepath.Join(base, "proj"), filepath.Join(base, "outside.json")
			_ = os.Mkdir(root, 0o755)
			if !dangling {
				mustWrite(t, outside, "untouched")
			}
			symlinkOrSkip(t, outside, filepath.Join(root, "f.json"))
			_, err := CreateFile(root, filepath.Join(root, "f.json"))
			requireRefused(t, err, "f.json", outside)
			if dangling && exists(outside) {
				t.Fatal("a file was made outside")
			}
			if !dangling && readString(t, outside) != "untouched" {
				t.Fatal("the outside file changed")
			}
		}
	})
	t.Run("a folder in the file's place is refused", func(t *testing.T) {
		root := t.TempDir()
		_ = os.Mkdir(filepath.Join(root, "f"), 0o755)
		_, err := CreateFile(root, filepath.Join(root, "f"))
		requireRefused(t, err, "f", "")
	})
	t.Run("the project folder is not a file", func(t *testing.T) {
		root := t.TempDir()
		_, err := CreateFile(root, root)
		requireRefused(t, err, "project folder", "")
	})
	t.Run("outside the project is refused", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "proj")
		_, err := CreateFile(root, filepath.Join(root, "..", "x.json"))
		requireRefused(t, err, "x.json", "")
	})
	t.Run("a failing folder walk is returned", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "f"), "x")
		_, err := CreateFile(root, filepath.Join(root, "f", "g.json"))
		requireRefused(t, err, "f", "")
	})
	t.Run("a failing Lstat of the file is returned", func(t *testing.T) {
		root := t.TempDir()
		withLstat(t, func(p string) (fs.FileInfo, error) {
			if strings.HasSuffix(p, "f.json") {
				return nil, &fs.PathError{Op: "lstat", Path: p, Err: syscall.EIO}
			}
			return os.Lstat(p)
		})
		_, err := CreateFile(root, filepath.Join(root, "f.json"))
		if !errors.Is(err, syscall.EIO) || strings.Contains(err.Error(), root) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a link put in place after the check is not followed", func(t *testing.T) {
		if noFollow == 0 {
			t.Skip("this platform has no O_NOFOLLOW; the Lstat check is its only guard")
		}
		base := t.TempDir()
		root, outside := filepath.Join(base, "proj"), filepath.Join(base, "outside.json")
		_ = os.Mkdir(root, 0o755)
		mustWrite(t, outside, "untouched")
		name := filepath.Join(root, "f.json")
		mustWrite(t, name, "plain")
		old := afterCheck
		afterCheck = func(p string) {
			_ = os.Remove(p)
			_ = os.Symlink(outside, p)
		}
		t.Cleanup(func() { afterCheck = old })
		_, err := CreateFile(root, name)
		requireRefused(t, err, "f.json", outside)
		if readString(t, outside) != "untouched" {
			t.Fatal("the outside file changed")
		}
	})
	t.Run("a link error from the open is a refusal", func(t *testing.T) {
		old := openFile
		openFile = func(string, int, fs.FileMode) (io.WriteCloser, error) {
			return nil, &fs.PathError{Op: "open", Path: "/abs", Err: syscall.ELOOP}
		}
		t.Cleanup(func() { openFile = old })
		root := t.TempDir()
		_, err := CreateFile(root, filepath.Join(root, "f.json"))
		requireRefused(t, err, "f.json", "/abs")
	})
	t.Run("another error from the open is returned without the absolute path", func(t *testing.T) {
		old := openFile
		openFile = func(string, int, fs.FileMode) (io.WriteCloser, error) {
			return nil, &fs.PathError{Op: "open", Path: "/abs", Err: syscall.EACCES}
		}
		t.Cleanup(func() { openFile = old })
		root := t.TempDir()
		_, err := CreateFile(root, filepath.Join(root, "f.json"))
		if !errors.Is(err, syscall.EACCES) || errors.Is(err, ErrNotPlain) || strings.Contains(err.Error(), "/abs") {
			t.Fatalf("got %v", err)
		}
	})
}

type fakeWriter struct {
	writeErr, closeErr error
	closed             bool
}

func (w *fakeWriter) Write(p []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return len(p), nil
}

func (w *fakeWriter) Close() error { w.closed = true; return w.closeErr }

func TestWriteFile(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "d", "f.txt")
	if err := WriteFile(root, name, func(w io.Writer) error { _, err := io.WriteString(w, "x"); return err }); err != nil {
		t.Fatal(err)
	}
	if readString(t, name) != "x" {
		t.Fatal("not written")
	}
	t.Run("a refusal is returned", func(t *testing.T) {
		err := WriteFile(root, filepath.Join(root, "..", "f"), func(io.Writer) error { return errors.New("not called") })
		requireRefused(t, err, "f", "")
	})
	withFake := func(t *testing.T, w *fakeWriter) {
		old := openFile
		openFile = func(string, int, fs.FileMode) (io.WriteCloser, error) { return w, nil }
		t.Cleanup(func() { openFile = old })
	}
	t.Run("the write error is returned and the file is closed", func(t *testing.T) {
		w := &fakeWriter{closeErr: errors.New("close failed too")}
		withFake(t, w)
		err := WriteFile(root, name, func(io.Writer) error { return errors.New("encode failed") })
		if err == nil || err.Error() != "encode failed" || !w.closed {
			t.Fatalf("got %v closed=%v", err, w.closed)
		}
	})
	t.Run("the close error is returned", func(t *testing.T) {
		w := &fakeWriter{closeErr: errors.New("close failed")}
		withFake(t, w)
		err := WriteFile(root, name, func(io.Writer) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "f.txt") || !strings.Contains(err.Error(), "close failed") {
			t.Fatalf("got %v", err)
		}
	})
}

func TestRemove(t *testing.T) {
	t.Run("removes a plain file and tolerates a missing one", func(t *testing.T) {
		root := t.TempDir()
		name := filepath.Join(root, "a", "f")
		_ = os.Mkdir(filepath.Join(root, "a"), 0o755)
		mustWrite(t, name, "x")
		for i := 0; i < 2; i++ {
			if err := Remove(root, name); err != nil {
				t.Fatal(err)
			}
		}
		if exists(name) {
			t.Fatal("not removed")
		}
	})
	t.Run("a missing folder above is not an error", func(t *testing.T) {
		root := t.TempDir()
		if err := Remove(root, filepath.Join(root, "no", "f")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a link in a folder above is refused and the outside file stays", func(t *testing.T) {
		base := t.TempDir()
		root, outside := filepath.Join(base, "proj"), filepath.Join(base, "outside")
		_ = os.MkdirAll(root, 0o755)
		_ = os.Mkdir(outside, 0o755)
		mustWrite(t, filepath.Join(outside, "f"), "keep")
		symlinkOrSkip(t, outside, filepath.Join(root, "l"))
		requireRefused(t, Remove(root, filepath.Join(root, "l", "f")), "l", outside)
		if readString(t, filepath.Join(outside, "f")) != "keep" {
			t.Fatal("the outside file changed")
		}
	})
	t.Run("a link in the file's place is refused", func(t *testing.T) {
		base := t.TempDir()
		root, outside := filepath.Join(base, "proj"), filepath.Join(base, "outside.json")
		_ = os.Mkdir(root, 0o755)
		mustWrite(t, outside, "keep")
		symlinkOrSkip(t, outside, filepath.Join(root, "f.json"))
		requireRefused(t, Remove(root, filepath.Join(root, "f.json")), "f.json", outside)
		if readString(t, outside) != "keep" {
			t.Fatal("the outside file changed")
		}
	})
	t.Run("outside the project, the project folder and a failing walk", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "f"), "x")
		requireRefused(t, Remove(root, filepath.Join(root, "..", "f")), "f", "")
		requireRefused(t, Remove(root, root), "project folder", "")
		requireRefused(t, Remove(root, filepath.Join(root, "f", "g")), "f", "")
	})
	t.Run("a failing Lstat of the file and a failing remove are returned", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "f"), "x")
		old := remove
		remove = func(string) error { return &fs.PathError{Op: "remove", Path: "/abs", Err: syscall.EBUSY} }
		t.Cleanup(func() { remove = old })
		err := Remove(root, filepath.Join(root, "f"))
		if !errors.Is(err, syscall.EBUSY) || strings.Contains(err.Error(), "/abs") {
			t.Fatalf("got %v", err)
		}
		withLstat(t, func(p string) (fs.FileInfo, error) {
			if strings.HasSuffix(p, "f") {
				return nil, &fs.PathError{Op: "lstat", Path: p, Err: syscall.EIO}
			}
			return os.Lstat(p)
		})
		if err = Remove(root, filepath.Join(root, "f")); !errors.Is(err, syscall.EIO) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestRemoveAll(t *testing.T) {
	t.Run("removes a folder with its content and tolerates a missing one", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "envs", "e")
		_ = os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
		mustWrite(t, filepath.Join(dir, "sub", "f"), "x")
		for i := 0; i < 2; i++ {
			if err := RemoveAll(root, dir); err != nil {
				t.Fatal(err)
			}
		}
		if exists(dir) || !exists(filepath.Join(root, "envs")) {
			t.Fatal("wrong removal")
		}
	})
	t.Run("a link in a folder above or in the folder's place is refused", func(t *testing.T) {
		base := t.TempDir()
		root, outside := filepath.Join(base, "proj"), filepath.Join(base, "outside")
		_ = os.MkdirAll(root, 0o755)
		_ = os.MkdirAll(filepath.Join(outside, "e"), 0o755)
		mustWrite(t, filepath.Join(outside, "e", "f"), "keep")
		symlinkOrSkip(t, outside, filepath.Join(root, "envs"))
		symlinkOrSkip(t, filepath.Join(outside, "e"), filepath.Join(root, "e"))
		requireRefused(t, RemoveAll(root, filepath.Join(root, "envs", "e")), "envs", outside)
		requireRefused(t, RemoveAll(root, filepath.Join(root, "e")), "e", outside)
		if readString(t, filepath.Join(outside, "e", "f")) != "keep" {
			t.Fatal("the outside file changed")
		}
	})
	t.Run("outside the project and the project folder", func(t *testing.T) {
		root := t.TempDir()
		requireRefused(t, RemoveAll(root, filepath.Join(root, "..", "x")), "x", "")
		requireRefused(t, RemoveAll(root, root), "project folder", "")
	})
	t.Run("a failing removal is returned", func(t *testing.T) {
		root := t.TempDir()
		_ = os.Mkdir(filepath.Join(root, "d"), 0o755)
		old := removeAll
		removeAll = func(string) error { return fmt.Errorf("busy") }
		t.Cleanup(func() { removeAll = old })
		err := RemoveAll(root, filepath.Join(root, "d"))
		if err == nil || err.Error() != "d: busy" {
			t.Fatalf("got %v", err)
		}
	})
}
