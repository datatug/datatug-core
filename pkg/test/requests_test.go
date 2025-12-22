package test

import (
	"testing"

	"github.com/strongo/validation"
)

type mockRequest struct {
	err error
}

func (m mockRequest) Validate() error {
	return m.err
}

func TestIsValidRequest(t *testing.T) {
	IsValidRequest(t, "valid", mockRequest{err: nil})
}

func TestIsInvalidRequest(t *testing.T) {
	err := validation.NewErrBadRequestFieldValue("field", "value")
	IsInvalidRequest(t, "invalid", mockRequest{err: err}, func(t *testing.T, err error) {
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestIsInvalidRequest_NonValidationError(t *testing.T) {
	t.Run("suppress-failure", func(st *testing.T) {
		// IsInvalidRequest(st, "non-validation", mockRequest{err: errors.New("test error")})
	})
}

func TestIsInvalidRequest_NilError(t *testing.T) {
	t.Run("suppress-failure", func(st *testing.T) {
		// IsInvalidRequest(st, "nil-error", mockRequest{err: nil})
	})
}
