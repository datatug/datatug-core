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

// checkQueryDirWritable requires the effective user to be able to add and
// remove entries in dir (write and search permission, access(2) with
// W_OK|X_OK). A writer checks it before committing, because the install
// renames into dir and a type change or delete removes from it: a folder
// made read-only would otherwise let the commit succeed and then fail the
// install - and so every later recovery - until someone fixed its
// permissions.
func checkQueryDirWritable(dir string) error {
	const wOK, xOK = 0x2, 0x1 // POSIX access(2) mode bits
	if err := syscall.Access(dir, wOK|xOK); err != nil {
		return fmt.Errorf("query folder %s does not accept new files (%w)", dir, err)
	}
	return nil
}
