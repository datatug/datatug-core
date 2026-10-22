//go:build !unix

package filestore

import "os"

// Environment recovery stores require verified private ownership. No Windows
// ACL ownership policy is implemented here; fail closed instead of treating
// missing POSIX metadata as authorization. This permits portable compilation,
// not Windows support for private environment transactions.
func environmentOwnedByCurrentUser(os.FileInfo) bool { return false }
