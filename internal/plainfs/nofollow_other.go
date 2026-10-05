//go:build !unix

package plainfs

// noFollow is empty where the platform has no O_NOFOLLOW (Windows, js,
// wasip1, plan9). There the Lstat check before every open is the only guard:
// it refuses links, and on Windows also junctions, which Lstat reports as
// neither a plain folder nor a plain file.
const noFollow = 0
