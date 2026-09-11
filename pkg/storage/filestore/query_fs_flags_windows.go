//go:build windows

package filestore

import "os"

// fileFlagsIssue reports a file with the read-only attribute, which Windows
// refuses to rename over or delete (Go reports it as the absence of the
// owner write bit). A folder's read-only attribute does not stop changes to
// its entries on Windows, so folders pass.
func fileFlagsIssue(_ string, info os.FileInfo) (reason string, bad bool) {
	if !info.IsDir() && info.Mode().Perm()&0o200 == 0 {
		return "is read-only (the read-only attribute is set)", true
	}
	return "", false
}
