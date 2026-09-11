package filestore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/datatug/datatug-core/pkg/storage"
)

const (
	// maxQueryFileSize caps each query metadata or body file the query
	// store reads or writes: 16 MiB. That is orders of magnitude above any
	// real query definition or body (the demo projects' largest is a few
	// KB). It bounds what one file can make a read hold; what a call that
	// reads many files may hold is bounded by maxQueryListingBytes. Every
	// write refuses content above it before staging, so a transaction never
	// commits a file that a later read or recovery would refuse.
	maxQueryFileSize = 16 << 20

	// maxQueryListingBytes caps the total bytes of query files - metadata
	// and bodies together - that one listing call reads: LoadQueries (one
	// folder) and LoadProject's query tree (every folder, one budget for the
	// whole walk). The capture endpoint serves these reads over HTTP, and a
	// git repository of identical 16 MiB bodies compresses to almost
	// nothing, so the per-file cap alone would let one call read an
	// unbounded total. Each file's size is checked against what is left of
	// the budget before the file is opened (queryReadBudget); once a file
	// would take the call past it, the call is refused with
	// errQueryListingTooLarge. A call that reads one query (LoadQuery,
	// LoadQueryRevision) reads at most two files, each within
	// maxQueryFileSize, and needs no budget. What a call holds in memory is
	// a small multiple of the bytes it reads (the decoded metadata and the
	// body as a string), not the bytes alone.
	maxQueryListingBytes = 256 << 20

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

// nonRegularEntryError refuses an entry that is not a regular file where
// the query store needs one - a symlink, directory, FIFO, device or socket -
// or a directory where a query file is to be removed. At one of a query's
// own file names that is a refusal of the query's location, so
// asQueryLocationError types it as *datatug.InvalidQueryLocationError, the
// error every other location refusal uses (review SF-D).
type nonRegularEntryError struct {
	path   string // the entry
	detail string // what it is and what is refused
}

func (e *nonRegularEntryError) Error() string { return e.path + " " + e.detail }

// asQueryLocationError returns err as a typed *datatug.InvalidQueryLocationError
// for (folderPath, id) when it refuses a non-regular entry at one of that
// query's own file names ("<id>.query.*"); any other error, including one
// about a transaction artifact, is returned unchanged.
func asQueryLocationError(folderPath, id string, err error) error {
	var entryErr *nonRegularEntryError
	if !errors.As(err, &entryErr) {
		return err
	}
	name := path.Base(filepath.ToSlash(entryErr.path))
	if !strings.HasPrefix(name, id+"."+storage.QueryFileSuffix+".") {
		return err
	}
	return invalidQueryLocation(folderPath, id, name+" "+entryErr.detail)
}

// requireRegularFile refuses anything but a regular file: a symlink (whose
// target could be anywhere), a directory, a FIFO (whose open or read can
// block forever), a device (/dev/zero never ends) or a socket.
func requireRegularFile(filePath string, info os.FileInfo) error {
	switch mode := info.Mode(); {
	case mode&os.ModeSymlink != 0:
		return &nonRegularEntryError{filePath, "is a symlink; refusing to use it"}
	case !mode.IsRegular():
		return &nonRegularEntryError{filePath, fmt.Sprintf("is not a regular file (%v); refusing to use it", mode.Type())}
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
	return readRegularFileBudgeted(filePath, maxSize, nil)
}

// errQueryListingTooLarge is wrapped by the error a listing call returns
// once the query files it reads would pass its maxQueryListingBytes budget.
var errQueryListingTooLarge = errors.New("the query files are too large to list in one call")

// queryReadBudget is one listing call's byte budget (maxQueryListingBytes),
// shared by the parallel readers of that call. A nil budget is unlimited;
// the per-file cap still applies.
type queryReadBudget struct {
	mu        sync.Mutex
	remaining int64
	limit     int64
}

// take charges n bytes read from filePath to the budget, refusing - and
// charging nothing - when fewer than n bytes are left.
func (b *queryReadBudget) take(filePath string, n int64) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if n > b.remaining {
		return fmt.Errorf("%w: reading %s (%d bytes) would pass the %d-byte budget for one listing, with %d bytes left; refusing to read it",
			errQueryListingTooLarge, filePath, n, b.limit, b.remaining)
	}
	b.remaining -= n
	return nil
}

// readRegularFileBudgeted is readRegularFileCapped that also charges the
// file to budget (nil: no budget): its size is charged after the Lstat and
// before the open, so a file that would pass the budget is never read; if
// the file grew between the Lstat and the read (it is still within
// maxSize), the extra bytes are charged too.
func readRegularFileBudgeted(filePath string, maxSize int64, budget *queryReadBudget) (data []byte, exists bool, err error) {
	info, exists, err := lstatRegularFile(filePath)
	if err != nil || !exists {
		return nil, exists, err
	}
	if info.Size() > maxSize {
		return nil, true, fmt.Errorf("%s is %d bytes, over the %d-byte limit; refusing to read it", filePath, info.Size(), maxSize)
	}
	if err := budget.take(filePath, info.Size()); err != nil {
		return nil, true, err
	}
	defer func() {
		if err == nil && int64(len(data)) > info.Size() {
			if err = budget.take(filePath, int64(len(data))-info.Size()); err != nil {
				data = nil
			}
		}
	}()
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
	return readQueryItemJSONBudgeted(filePath, dst, nil)
}

// readQueryItemJSONBudgeted is readQueryItemJSON charging the file to a
// listing call's budget (see maxQueryListingBytes); nil means none.
func readQueryItemJSONBudgeted(filePath string, dst any, budget *queryReadBudget) error {
	b, exists, err := readRegularFileBudgeted(filePath, maxQueryFileSize, budget)
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
		return &nonRegularEntryError{filePath, "is a directory; refusing to remove it as a query file"}
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

// chmodDirNoFollow sets dir's permissions through a descriptor opened with
// openNoFollowFlags (no symlink, no blocking on a FIFO), after checking
// that the opened entry is the very directory info (from Lstat) describes.
// os.Chmod would follow a symlink swapped in after the Lstat and change its
// target instead.
func chmodDirNoFollow(dir string, info os.FileInfo, perm os.FileMode) error {
	f, err := os.OpenFile(dir, os.O_RDONLY|openNoFollowFlags, 0)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	if !opened.IsDir() || !os.SameFile(info, opened) {
		return fmt.Errorf("%s changed while it was being opened", dir)
	}
	return f.Chmod(perm)
}
