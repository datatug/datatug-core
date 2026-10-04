//go:build linux

package filestore

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestLinuxAttrsIssue(t *testing.T) {
	for _, tc := range []struct {
		name             string
		attributes, mask uint64
		bad              bool
	}{
		{"none", 0, unix.STATX_ATTR_IMMUTABLE | unix.STATX_ATTR_APPEND, false},
		{"immutable", unix.STATX_ATTR_IMMUTABLE, unix.STATX_ATTR_IMMUTABLE, true},
		{"append-only", unix.STATX_ATTR_APPEND, unix.STATX_ATTR_APPEND, true},
		{"set but not supported by the file system", unix.STATX_ATTR_IMMUTABLE, 0, false},
		{"unrelated attribute", unix.STATX_ATTR_COMPRESSED, unix.STATX_ATTR_COMPRESSED, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason, bad := linuxAttrsIssue(tc.attributes, tc.mask)
			if bad != tc.bad || (reason != "") != tc.bad {
				t.Errorf("expected bad=%v, got %q, %v", tc.bad, reason, bad)
			}
		})
	}
}
