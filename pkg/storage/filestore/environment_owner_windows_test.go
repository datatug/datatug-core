//go:build windows

package filestore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsEnvironmentOwnershipFailsClosed(t *testing.T) {
	if environmentOwnedByCurrentUser(nil) {
		t.Fatal("missing ownership evidence admitted")
	}
	path := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(path, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if environmentOwnedByCurrentUser(info) {
		t.Fatal("Windows mode bits are not an ownership grant")
	}
	directory, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if environmentOwnedByCurrentUser(directory) {
		t.Fatal("Windows directory mode is not an ownership grant")
	}
}
