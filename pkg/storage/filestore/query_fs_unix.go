//go:build unix

package filestore

import (
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
