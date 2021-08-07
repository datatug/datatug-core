package test

import (
	"fmt"
	"github.com/strongo/validation"
	"testing"
)

type request interface {
	Validate() error
}

func ValidRequest(t *testing.T, name string, r request) {
	t.Run(name, func(t *testing.T) {
		if err := r.Validate(); err != nil {
			t.Error(fmt.Sprintf("unexpected error of type %T for request %+v", err, r), err)
		}
	})
}

func InvalidRequest(t *testing.T, name string, r request) {
	t.Run(name, func(t *testing.T) {
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
	})
}
