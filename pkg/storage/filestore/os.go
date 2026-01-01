package filestore

import (
	"io"
	"os"
)

var standardOsOpen = func(name string) (io.ReadCloser, error) {
	return os.Open(name)
}

var standardOsStat = os.Stat

var standardOsMkdirAll = os.MkdirAll

var standardOsCreate = func(name string) (io.WriteCloser, error) {
	return os.Create(name)
}

var (
	osOpen     = standardOsOpen
	osStat     = standardOsStat
	osMkdirAll = standardOsMkdirAll
	osCreate   = standardOsCreate
)
