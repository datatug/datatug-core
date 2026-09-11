package filestore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"runtime"
)

const (
	// maxQueryFileSize caps every query metadata or body file the query
	// store reads or writes: 16 MiB. That is orders of magnitude above any
	// real query definition or body (the demo projects' largest is a few
	// KB), yet small enough that a pathological file cannot exhaust memory
	// on a read - the capture endpoint exposes these reads over HTTP. Every
	// write refuses content above it before staging, so a transaction never
	// commits a file that a later read or recovery would refuse.
	maxQueryFileSize = 16 << 20

	// maxQueryTxnJournalSize bounds the transaction journal (the plan's
	// "durable bounded journal"). A journal holds one validated query
	// identity, two SHA-256 hashes and derived file names, so even a folder
	// path at the operating system's path-length limit stays far below it.
	maxQueryTxnJournalSize = 64 << 10
)

// fileOwnedByCurrentUser is ownedByCurrentUser (query_fs_unix.go,
// query_fs_other.go). It is a variable only so tests can simulate a file
// owned by another user, which an unprivileged test cannot create.
var fileOwnedByCurrentUser = ownedByCurrentUser

// requireRegularFile refuses anything but a regular file: a symlink (whose
// target could be anywhere), a directory, a FIFO (whose open or read can
// block forever), a device (/dev/zero never ends) or a socket.
func requireRegularFile(filePath string, info os.FileInfo) error {
	switch mode := info.Mode(); {
	case mode&os.ModeSymlink != 0:
		return fmt.Errorf("%s is a symlink; refusing to use it", filePath)
	case !mode.IsRegular():
		return fmt.Errorf("%s is not a regular file (%v); refusing to use it", filePath, mode.Type())
	}
	return nil
}

// lstatRegularFile Lstats filePath and requires a regular file. It returns
// exists=false with a nil error when nothing is there.
func lstatRegularFile(filePath string) (info os.FileInfo, exists bool, err error) {
	info, err = os.Lstat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if err := requireRegularFile(filePath, info); err != nil {
		return info, true, err
	}
	return info, true, nil
}

// readRegularFileCapped reads filePath only when it is a regular file of at
// most maxSize bytes, returning exists=false with a nil error when nothing
// is there. It checks the entry with Lstat, opens it with
// openNoFollowFlags (no symlink, no blocking on a FIFO) and then requires
// the opened file to be the very regular file it checked (os.SameFile), so
// swapping the entry between the check and the open cannot redirect the
// read. The read itself is limited to maxSize+1 bytes, so a file that grows
// while it is read is refused rather than read without bound.
func readRegularFileCapped(filePath string, maxSize int64) (data []byte, exists bool, err error) {
	info, exists, err := lstatRegularFile(filePath)
	if err != nil || !exists {
		return nil, exists, err
	}
	if info.Size() > maxSize {
		return nil, true, fmt.Errorf("%s is %d bytes, over the %d-byte limit; refusing to read it", filePath, info.Size(), maxSize)
	}
	f, err := os.OpenFile(filePath, os.O_RDONLY|openNoFollowFlags, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, true, fmt.Errorf("failed to open %s: %w", filePath, err)
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil {
		return nil, true, fmt.Errorf("failed to inspect %s: %w", filePath, err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, true, fmt.Errorf("%s changed while it was being opened; refusing to read it", filePath)
	}
	data, err = io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, true, fmt.Errorf("failed to read %s: %w", filePath, err)
	}
	if int64(len(data)) > maxSize {
		return nil, true, fmt.Errorf("%s grew over the %d-byte limit while it was read; refusing it", filePath, maxSize)
	}
	return data, true, nil
}

// readQueryItemJSON is the legacy query loaders' JSON reader (the query
// store's fsProjectItemsStore.readItemJSON): readJSONFile's decoding - the
// first JSON value in the file, exactly as before - over
// readRegularFileCapped, so a legacy LoadQuery, LoadQueries or project-tree
// load reads only a regular file within maxQueryFileSize, never through a
// symlink, FIFO or device. A missing file keeps readJSONFile's error shape
// (an *fs.PathError wrapping fs.ErrNotExist).
func readQueryItemJSON(filePath string, dst any) error {
	b, exists, err := readRegularFileCapped(filePath, maxQueryFileSize)
	if err != nil {
		return err
	}
	if !exists {
		return &fs.PathError{Op: "open", Path: filePath, Err: fs.ErrNotExist}
	}
	return json.NewDecoder(bytes.NewReader(b)).Decode(dst)
}

// checkRemovableQueryFile refuses a query target path that exists as a
// directory: removing a pair must never remove a directory, or fail on
// one halfway through. Any other entry may be removed - os.Remove removes
// a symlink or FIFO itself, never what a symlink points to.
func checkRemovableQueryFile(filePath string) error {
	info, err := os.Lstat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory; refusing to remove it as a query file", filePath)
	}
	return nil
}

// checkTxnArtifact vets an entry of the private transaction directory (the
// journal, journal.tmp, a staged file) before it is trusted in any way -
// read, installed, or removed as an uncommitted leftover. It must be a
// regular file, never a symlink, FIFO, device, socket or directory; it
// must be owned by the effective user running this process; and, where
// POSIX permission bits are meaningful (not Windows, where Go reports
// synthetic ones), no more permissive than maxPerm. It returns
// exists=false with a nil error when nothing is there.
func checkTxnArtifact(filePath string, maxPerm os.FileMode) (exists bool, err error) {
	info, exists, err := lstatRegularFile(filePath)
	if err != nil || !exists {
		return exists, err
	}
	if !fileOwnedByCurrentUser(info) {
		return true, fmt.Errorf("%s is owned by another user; refusing to use it", filePath)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&^maxPerm != 0 {
		return true, fmt.Errorf("%s has overly broad permissions %v; refusing to use it", filePath, info.Mode().Perm())
	}
	return true, nil
}
