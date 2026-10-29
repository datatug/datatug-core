package plainfs

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type publicationFile interface {
	io.WriteCloser
	Sync() error
}

// These seams retain real rooted filesystem behavior outside failure tests.
var (
	publishOpenFile = func(root *os.Root, name string) (publicationFile, error) {
		return root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|noFollow, 0o600)
	}
	publishLink       = (*os.Root).Link
	publishRemove     = (*os.Root).Remove
	publishLstat      = (*os.Root).Lstat
	publishRootStat   = os.Stat
	publishMkdir      = (*os.Root).Mkdir
	publishOpenParent = (*os.Root).OpenRoot
	publishSyncDir    = func(root *os.Root) error {
		f, err := root.Open(".")
		if err != nil {
			return err
		}
		return errors.Join(f.Sync(), f.Close())
	}
)

type publicationDirectory struct {
	name string
	info fs.FileInfo
}

// Verify the logical path as well as the opened roots: os.Root deliberately
// keeps referring to a directory after it moves, and follows internal links.
func verifyPublicationDirectories(rootPath string, root *os.Root, parents []publicationDirectory) error {
	info, err := publishRootStat(rootPath)
	if err != nil {
		return wrap(rootName, err)
	}
	if !os.SameFile(info, parents[0].info) {
		return &refusal{rootName, "changed directory identity"}
	}
	for _, parent := range parents[1:] {
		info, err := publishLstat(root, parent.name)
		if err != nil {
			return wrap(display(parent.name), err)
		}
		if reason := folderIssue(info); reason != "" {
			return &refusal{display(parent.name), reason}
		}
		if !os.SameFile(info, parent.info) {
			return &refusal{display(parent.name), "changed directory identity"}
		}
	}
	return nil
}

// PublishFile writes and closes a private stage, then publishes the complete
// file with an atomic hard-link operation that never replaces an entry.
// Paths remain confined to root and every existing parent must be plain.
// Cleanup removes empty created directories only while their observed identities
// still occupy the same plain path. A replaced/moved directory is preserved.
// Errors after the link may leave a committed target; no rollback is claimed.
// Namespace checks are not a transaction with uncoordinated external renames;
// callers must coordinate project-directory mutations. A detected post-link
// identity change returns an error rather than acknowledging the wrong path.
func PublishFile(ctx context.Context, rootPath, filePath string, data []byte, authorize func() error) (err error) {
	rel, err := relInside(rootPath, filePath)
	if err != nil {
		return err
	}
	if rel == "." {
		return &refusal{rootName, "is a folder, not a file"}
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if err := ctx.Err(); err != nil {
		return err
	}
	rootInfo, err := publishLstat(root, ".")
	if err != nil {
		return wrap(rootName, err)
	}
	parents := []publicationDirectory{{name: ".", info: rootInfo}}
	var created []int
	defer func() {
		// Never remove a replacement empty directory by its old pathname.
		for i := len(created) - 1; i >= 0; i-- {
			index := created[i]
			if verifyPublicationDirectories(rootPath, root, parents[:index+1]) == nil {
				_ = root.Remove(parents[index].name)
			}
		}
	}()
	dir := ""
	for _, part := range strings.Split(filepath.Dir(rel), string(filepath.Separator)) {
		dir = filepath.Join(dir, part)
		made := false
		info, e := publishLstat(root, dir)
		if errors.Is(e, fs.ErrNotExist) {
			e = publishMkdir(root, dir, folderPerm)
			if e == nil {
				made = true
			} else if !errors.Is(e, fs.ErrExist) {
				return wrap(dir, e)
			}
			info, e = publishLstat(root, dir)
		}
		if e != nil {
			return wrap(dir, e)
		}
		if reason := folderIssue(info); reason != "" {
			return &refusal{display(dir), reason}
		}
		parents = append(parents, publicationDirectory{name: dir, info: info})
		if made {
			created = append(created, len(parents)-1)
		}
	}
	parent, err := publishOpenParent(root, filepath.Dir(rel))
	if err != nil {
		return wrap(display(filepath.Dir(rel)), err)
	}
	defer func() { _ = parent.Close() }()
	openedInfo, err := publishLstat(parent, ".")
	if err != nil {
		return wrap(display(filepath.Dir(rel)), err)
	}
	if !os.SameFile(openedInfo, parents[len(parents)-1].info) {
		return &refusal{display(filepath.Dir(rel)), "changed directory identity"}
	}
	stage := ".datatug-create-" + rand.Text() + ".tmp"
	f, err := publishOpenFile(parent, stage)
	if err != nil {
		return wrap(stage, err)
	}
	defer func() { err = errors.Join(err, publishRemove(parent, stage)) }()
	_, err = io.Copy(f, bytes.NewReader(data))
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if authorize != nil {
		if err := authorize(); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := verifyPublicationDirectories(rootPath, root, parents); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Link is the final absent-target decision, not an existence-check followed
	// by truncation or replacing rename. A racing valid file is preserved.
	if err := publishLink(parent, stage, filepath.Base(rel)); err != nil {
		return wrap(display(rel), err)
	}
	if err := publishSyncDir(parent); err != nil {
		return err
	}
	if err := verifyPublicationDirectories(rootPath, root, parents); err != nil {
		return err
	}
	return ctx.Err()
}
