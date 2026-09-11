//go:build !unix

package filestore

import "os"

// openNoFollowFlags is empty where the platform has no O_NOFOLLOW/O_NONBLOCK
// (Windows, js, wasip1, plan9). The Lstat type check before every open, and
// the fstat/os.SameFile check after it, still apply.
const openNoFollowFlags = 0

// ownedByCurrentUser accepts every file where os.FileInfo carries no POSIX
// owner to compare (Windows ACLs are not modelled by it). The regular-file
// and derived-name checks still apply there.
func ownedByCurrentUser(os.FileInfo) bool { return true }

// checkQueryDirWritable accepts every directory where there is no POSIX
// access(2) to ask (Windows ACLs are not modelled by it); an install that
// then fails leaves the committed transaction for recovery to retry.
func checkQueryDirWritable(string) error { return nil }
