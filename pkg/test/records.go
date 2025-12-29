package test

import (
	"fmt"
	"testing"

	"github.com/strongo/validation"
)

type record interface {
	Validate() error
}

type TestingT interface {
	Helper()
	Run(name string, f func(t *testing.T)) bool
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
}

func IsValidRecord(t TestingT, name string, r record) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		isValidRecord(t, r)
	})
}

func isValidRecord(t TestingT, r record) {
	t.Helper()
	if err := r.Validate(); err != nil {
		t.Error(fmt.Sprintf("unexpected error of type %T for a valid record of type %T: %+v", err, r, r), err)
	}
}

func IsInvalidRecord(t TestingT, name string, r record, errorValidators ...func(t *testing.T, err error)) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		isInvalidRecord(t, r, errorValidators...)
	})
}

func isInvalidRecord(t TestingT, r record, errorValidators ...func(t *testing.T, err error)) {
	t.Helper()
	err := r.Validate()
	if err == nil {
		t.Errorf("expected an error but got nil for r: %+v", r)
	}
	if !validation.IsValidationError(err) {
		t.Errorf("returned error is not a validation error: %T: %v; record: %+v", err, err, r)
	}
	if !validation.IsBadRecordError(err) {
		t.Errorf("returned error is not a bad record error: %T: %v; record: %+v", err, err, r)
	}
	for _, errValidator := range errorValidators {
		// Since validators take *testing.T, we can only call them if t is *testing.T
		if tt, ok := t.(*testing.T); ok {
			errValidator(tt, err)
		} else {
			// For mockTestingT, we can't easily pass a *testing.T that won't fail
			// but we can just skip calling it in mock tests or provide a way
			t.Errorf("cannot call error validator with mock TestingT")
		}
	}
}
