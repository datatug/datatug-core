//go:build unix

package filestore

import (
	"fmt"
	"os"
	"syscall"
)

// openNoFollowFlags are added to every open of a query-store file whose
// type was just checked with Lstat. O_NOFOLLOW refuses a symlink swapped in
// after the check; O_NONBLOCK keeps a FIFO swapped in after the check from
// blocking the open (it is then refused by the fstat check that follows).
// Neither changes how an ordinary regular file is read.
const openNoFollowFlags = syscall.O_NOFOLLOW | syscall.O_NONBLOCK

// ownedByCurrentUser reports whether info (from Lstat) belongs to the
// effective user running this process.
func ownedByCurrentUser(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int64(st.Uid) == int64(os.Geteuid())
}

// checkQueryDirWritable requires the user running DataTug to be able to add
// and remove entries in dir: write and search permission, access(2) with
// W_OK|X_OK. access(2) checks the real user and group IDs - on macOS and
// Linux alike - not the effective ones ownedByCurrentUser compares with;
// the two are the same unless the process runs setuid or setgid, which
// DataTug does not (review N1). For root, access(2) grants W_OK just as
// rename(2) then allows. On NFS, SMB or FUSE its answer can disagree with
// what the server enforces (review N2): a false "not writable" refuses the
// write as an ordinary error, and a false "writable" is caught by the
// install, which rolls back or scopes the failure to its query
// (finishQueryTransaction, recoverQueryTransactions). checkQueryTxnTargets
// runs it - so both the writer before its commit and recovery before it
// touches anything - through checkQueryDirAcceptsChanges, which adds the
// file-flag half.
func checkQueryDirWritable(dir string) error {
	const wOK, xOK = 0x2, 0x1 // POSIX access(2) mode bits
	if err := syscall.Access(dir, wOK|xOK); err != nil {
		return fmt.Errorf("query folder %s does not accept new files (%w)", dir, err)
	}
	return nil
}
