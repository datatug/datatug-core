//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package filestore

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
)

// BSD file flags (chflags(2)) that make rename(2) over a file, or unlink(2)
// of it, fail even when its folder is writable. The values are the 4.4BSD
// ones every BSD and macOS share; the no-unlink flags exist on some only.
const (
	bsdUserImmutable   = 0x00000002 // UF_IMMUTABLE: chflags uchg, Finder's "Locked"
	bsdUserAppend      = 0x00000004 // UF_APPEND: chflags uappnd
	bsdSystemImmutable = 0x00020000 // SF_IMMUTABLE: chflags schg
	bsdSystemAppend    = 0x00040000 // SF_APPEND: chflags sappnd
	bsdUserNoUnlink    = 0x00000010 // UF_NOUNLINK (FreeBSD, DragonFly)
	bsdSystemNoUnlink  = 0x00100000 // SF_NOUNLINK (macOS, FreeBSD, DragonFly)
)

var bsdNoReplaceFlags = func() uint32 {
	flags := uint32(bsdUserImmutable | bsdUserAppend | bsdSystemImmutable | bsdSystemAppend)
	switch runtime.GOOS {
	case "darwin":
		flags |= bsdSystemNoUnlink
	case "freebsd", "dragonfly":
		flags |= bsdUserNoUnlink | bsdSystemNoUnlink
	}
	return flags
}()

// fileFlagsIssue reports why the entry info (from Lstat) describes cannot
// be renamed over or removed because of its file flags - immutable,
// append-only or no-unlink - or ("", false) when no such flag is set.
func fileFlagsIssue(_ string, info os.FileInfo) (reason string, bad bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false
	}
	if flags := uint32(st.Flags) & bsdNoReplaceFlags; flags != 0 {
		return fmt.Sprintf("is locked (file flags %#x: immutable, append-only or no-unlink - Finder's Locked checkbox or chflags)", flags), true
	}
	return "", false
}
