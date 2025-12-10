package datatug

import "errors"

// ErrProjectDoesNotExist is an error that indicates a project does not exist
var ErrProjectDoesNotExist = errors.New("project does not exist")

// ProjectDoesNotExist reports if an error is a wrapper around ErrProjectDoesNotExist
func ProjectDoesNotExist(err error) bool {
	return errors.Is(err, ErrProjectDoesNotExist)
}
