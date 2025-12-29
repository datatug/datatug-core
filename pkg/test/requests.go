package test

import (
	"fmt"
	"testing"

	"github.com/strongo/validation"
)

type request interface {
	Validate() error
}

func IsValidRequest(t TestingT, name string, r request) {
	t.Run(name, func(t *testing.T) {
		isValidRequest(t, r)
	})
}

func isValidRequest(t TestingT, r request) {
	if err := r.Validate(); err != nil {
		t.Error(fmt.Sprintf("unexpected error of type %T for request %+v", err, r), err)
	}
}

func IsInvalidRequest(t TestingT, name string, r request, errorValidators ...func(t *testing.T, err error)) {
	t.Run(name, func(t *testing.T) {
		isInvalidRequest(t, r, errorValidators...)
	})
}

func isInvalidRequest(t TestingT, r request, errorValidators ...func(t *testing.T, err error)) {
	err := r.Validate()
	if err == nil {
		t.Errorf("expected an error but got nil for r: %+v", r)
	}
	if !validation.IsValidationError(err) {
		t.Errorf("returned error is not a validation error: %T: %v; request: %+v", err, err, r)
	}
	if !validation.IsBadRequestError(err) {
		t.Errorf("returned error is not a bad request error: %T: %v; request: %+v", err, err, r)
	}
	for _, errValidator := range errorValidators {
		if tt, ok := t.(*testing.T); ok {
			errValidator(tt, err)
		} else {
			t.Errorf("cannot call error validator with mock TestingT")
		}
	}
}
