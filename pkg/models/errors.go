package models

import "errors"

// ErrProjectDoesNotExist is an error that indicates project does not exists
var ErrProjectDoesNotExist = errors.New("project does not exist")

// ProjectDoesNotExist retports if an error is a wrapper around ErrProjectDoesNotExist
func ProjectDoesNotExist(err error) bool {
	return errors.Is(err, ErrProjectDoesNotExist)
}
