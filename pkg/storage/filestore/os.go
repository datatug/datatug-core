package filestore

import (
	"io"
	"os"
)

var standardOsOpen = func(name string) (io.ReadCloser, error) {
	return os.Open(name)
}

var standardOsStat = os.Stat

var (
	osOpen = standardOsOpen
	osStat = standardOsStat
)
