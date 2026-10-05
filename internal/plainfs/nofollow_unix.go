//go:build unix

package plainfs

import "syscall"

// noFollow is added to every open for writing. O_NOFOLLOW refuses a link put
// in the file's place after the Lstat check; O_NONBLOCK keeps a pipe put
// there from blocking the open.
const noFollow = syscall.O_NOFOLLOW | syscall.O_NONBLOCK
