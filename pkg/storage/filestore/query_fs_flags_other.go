//go:build !(darwin || dragonfly || freebsd || netbsd || openbsd || linux || windows)

package filestore

import "os"

// platformFileFlagsIssue knows no flags that forbid replacing a file on this
// platform; the install then fails without a torn result if one does.
func platformFileFlagsIssue(string, os.FileInfo) (reason string, bad bool) { return "", false }
