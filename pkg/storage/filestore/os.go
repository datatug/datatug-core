package filestore

import (
	"io"
	"os"
)

var standardOsOpen = func(name string) (io.ReadCloser, error) {
	return os.Open(name)
}

var osOpen = standardOsOpen
