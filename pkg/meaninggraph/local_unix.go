//go:build unix

package meaninggraph

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func validateFixtureRoot(root string) error {
	fd, err := openFixtureRoot(root)
	if err != nil {
		return err
	}
	return unix.Close(fd)
}

// Open every root component relative to an already-open directory. A symlink
// in the configured root's ancestors must not bypass the fixture boundary.
func openFixtureRoot(root string) (int, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return -1, err
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, part := range splitPath(absolute[1:]) {
		if part == "" {
			continue
		}
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return -1, e
		}
		fd = next
	}
	return fd, nil
}
func openFixtureFile(root, path string) (io.ReadCloser, error) {
	fd, err := openFixtureRoot(root)
	if err != nil {
		return nil, err
	}
	parts := splitPath(path)
	for i, part := range parts {
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		if i < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, e := unix.Openat(fd, part, flags, 0)
		unix.Close(fd)
		if e != nil {
			return nil, e
		}
		fd = next
	}
	return regularFixtureFile(os.NewFile(uintptr(fd), "retained fixture"))
}

func regularFixtureFile(file *os.File) (io.ReadCloser, error) {
	st, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !st.Mode().IsRegular() {
		file.Close()
		return nil, errors.New("retained fixture must be a regular file")
	}
	return file, nil
}
