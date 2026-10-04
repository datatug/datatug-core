//go:build linux

package filestore

import (
	"os"

	"golang.org/x/sys/unix"
)

// platformFileFlagsIssue reports why filePath cannot be renamed over or removed
// because of its Linux inode attributes - immutable or append-only
// (chattr +i/+a) - or ("", false) when neither is set. It reads them with
// statx(2) without following a symlink and without opening the file. Where
// statx is unavailable (a kernel before 4.11, a seccomp filter) or the file
// system does not report the attributes, nothing is known and nothing is
// refused here; the install then fails without a torn result.
func platformFileFlagsIssue(filePath string, _ os.FileInfo) (reason string, bad bool) {
	var stx unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, filePath, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MODE, &stx); err != nil {
		return "", false
	}
	return linuxAttrsIssue(stx.Attributes, stx.Attributes_mask)
}

// linuxAttrsIssue is the decision part of platformFileFlagsIssue, split out
// so a test can feed it attributes without chattr and root: it refuses only
// an attribute the file system reports as supported (mask).
func linuxAttrsIssue(attributes, mask uint64) (reason string, bad bool) {
	const locked = unix.STATX_ATTR_IMMUTABLE | unix.STATX_ATTR_APPEND
	if attributes&mask&locked != 0 {
		return "is immutable or append-only (chattr +i or +a)", true
	}
	return "", false
}
