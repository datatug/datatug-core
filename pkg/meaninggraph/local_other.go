//go:build !unix

package meaninggraph

import (
	"errors"
	"io"
)

func validateFixtureRoot(string) error {
	return errors.New("safe fixture transport is unavailable on this platform")
}
func openFixtureFile(string, string) (io.ReadCloser, error) {
	return nil, errors.New("safe fixture transport is unavailable on this platform")
}
