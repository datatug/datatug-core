//go:build unix

package filestore

import (
	"os"
	"syscall"
)

// Preserve the environment transaction's real-UID ownership policy.
func environmentOwnedByCurrentUser(info os.FileInfo) bool {
	owner, ok := info.Sys().(*syscall.Stat_t)
	return ok && owner.Uid == uint32(os.Getuid())
}
