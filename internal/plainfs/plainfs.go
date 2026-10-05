// Package plainfs writes, removes and creates folders inside a project folder
// only through plain files and plain folders.
//
// A project folder may have been cloned from someone else, so nothing below
// it can be trusted to be what it looks like. Every function here takes the
// project folder (root, trusted as given) and a path inside it, walks from
// root down to the path with Lstat, and refuses a folder that is a link or
// is not a plain folder, and a target that exists and is not a plain file.
// A file is opened so that a link put in its place after the check is not
// followed where the platform has O_NOFOLLOW (every Unix); on Windows, which
// has none, the Lstat check alone applies, and it also refuses a junction.
//
// A refusal wraps ErrNotPlain and names the path inside the project; it never
// says where a link leads. Reads are not covered by this package.
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
)

// ErrNotPlain is wrapped by every refusal: a path outside the project, a
// link, or an entry of another kind where a plain file or folder is needed.
var ErrNotPlain = errors.New("not a plain file or folder of the project")

// refusal is the error behind ErrNotPlain: the path inside the project and
// what is wrong with it.
type refusal struct{ rel, reason string }

func (r *refusal) Error() string {
	return r.rel + ": " + r.reason + "; refusing to use it"
}

func (r *refusal) Unwrap() error { return ErrNotPlain }

// The operating-system calls, as variables so that tests can inject the
// failures and the entries (a link on a platform that cannot make one) that a
// real folder cannot produce on demand.
var (
	lstat     = os.Lstat
	mkdir     = os.Mkdir
	mkdirAll  = os.MkdirAll
	remove    = os.Remove
	removeAll = os.RemoveAll
	openFile  = func(name string, flag int, perm fs.FileMode) (io.WriteCloser, error) {
		return os.OpenFile(name, flag, perm)
	}
	// afterCheck runs between the Lstat of a file and its open; a test uses it
	// to put a link in the file's place at exactly that moment.
	afterCheck = func(string) {}
)

const (
	folderPerm = 0o777
	filePerm   = 0o666
	rootName   = "the project folder"
)

// MkdirAll makes dir, which must be root or inside it, and every missing
// folder between them, refusing any existing one that is not a plain folder.
func MkdirAll(root, dir string) error {
	rel, err := relInside(root, dir)
	if err != nil {
		return err
	}
	_, err = walkFolders(root, rel, true)
	return err
}

// CreateFile opens filePath, which must be inside root, for writing: it makes
// the missing folders above it, refuses a target that exists and is not a
// plain file, and creates or truncates the file without following a link.
func CreateFile(root, filePath string) (io.WriteCloser, error) {
	rel, err := relInside(root, filePath)
	if err != nil {
		return nil, err
	}
	if rel == "." {
		return nil, &refusal{rootName, "is a folder, not a file"}
	}
	if _, err = walkFolders(root, filepath.Dir(rel), true); err != nil {
		return nil, err
	}
	full := filepath.Join(root, rel)
	if err = checkFile(full, rel); err != nil {
		return nil, err
	}
	afterCheck(full)
	f, err := openFile(full, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|noFollow, filePerm)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, &refusal{display(rel), "is a link, not a plain file"}
		}
		return nil, wrap(rel, err)
	}
	return f, nil
}

// WriteFile is CreateFile followed by write and a Close; it returns the
// error of the write, or else the error of the close.
func WriteFile(root, filePath string, write func(io.Writer) error) (err error) {
	f, err := CreateFile(root, filePath)
	if err != nil {
		return err
	}
	if err = write(f); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		rel, _ := relInside(root, filePath) // CreateFile has just accepted it
		return wrap(display(rel), err)
	}
	return nil
}

// Remove deletes the plain file filePath, which must be inside root. A file
// that is not there, or a folder above it that is not there, is not an error.
func Remove(root, filePath string) error {
	rel, err := relInside(root, filePath)
	if err != nil {
		return err
	}
	if rel == "." {
		return &refusal{rootName, "is a folder, not a file"}
	}
	found, err := walkFolders(root, filepath.Dir(rel), false)
	if err != nil || !found {
		return err
	}
	full := filepath.Join(root, rel)
	if err = checkFile(full, rel); err != nil {
		return err
	}
	if err = remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return wrap(rel, err)
	}
	return nil
}

// RemoveAll deletes the plain folder dir, which must be inside root, with
// everything in it. A folder that is not there is not an error; the project
// folder itself is never removed.
func RemoveAll(root, dir string) error {
	rel, err := relInside(root, dir)
	if err != nil {
		return err
	}
	if rel == "." {
		return &refusal{rootName, "cannot be removed"}
	}
	found, err := walkFolders(root, rel, false)
	if err != nil || !found {
		return err
	}
	if err = removeAll(filepath.Join(root, rel)); err != nil {
		return wrap(rel, err)
	}
	return nil
}

// relInside returns p relative to root, or a refusal when p is not root or
// inside it. The check is on the paths as written, before any Lstat.
func relInside(root, p string) (string, error) {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(p))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", &refusal{filepath.ToSlash(filepath.Clean(p)), "is outside the project"}
	}
	return rel, nil
}

// walkFolders checks that every folder from root down to rel (a path
// relative to root, "." for root itself) is a plain folder. With create each
// missing one is made, root itself with MkdirAll since it is the caller's
// own folder; without it the walk stops at the first missing folder and
// reports found=false.
func walkFolders(root, rel string, create bool) (found bool, err error) {
	if create {
		if err = mkdirAll(root, folderPerm); err != nil {
			return false, wrap(rootName, err)
		}
	}
	if rel == "." {
		return true, nil
	}
	dir := root
	parts := strings.Split(rel, string(filepath.Separator))
	for i, name := range parts {
		dir = filepath.Join(dir, name)
		shown := display(filepath.Join(parts[:i+1]...))
		info, err := lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			if !create {
				return false, nil
			}
			if err = mkdir(dir, folderPerm); err != nil && !errors.Is(err, fs.ErrExist) {
				return false, wrap(shown, err)
			}
			info, err = lstat(dir)
		}
		if err != nil {
			return false, wrap(shown, err)
		}
		if reason := folderIssue(info); reason != "" {
			return false, &refusal{shown, reason}
		}
	}
	return true, nil
}

// checkFile refuses full when it exists and is not a plain file.
func checkFile(full, rel string) error {
	info, err := lstat(full)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return wrap(display(rel), err)
	}
	mode := info.Mode()
	switch {
	case mode&fs.ModeSymlink != 0:
		return &refusal{display(rel), "is a link, not a plain file"}
	case !mode.IsRegular():
		return &refusal{display(rel), "is not a plain file"}
	}
	return nil
}

func folderIssue(info fs.FileInfo) string {
	mode := info.Mode()
	switch {
	case mode&fs.ModeSymlink != 0:
		return "is a link, not a plain folder"
	case !mode.IsDir():
		return "is not a plain folder"
	}
	return ""
}

// display is rel as the refusal shows it: slash-separated.
func display(rel string) string { return filepath.ToSlash(rel) }

// wrap prefixes err with the path inside the project and drops the absolute
// path the operating system's error carries, keeping the cause for errors.Is.
func wrap(shown string, err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return fmt.Errorf("%s: %w", shown, err)
}
