package test

import (
	"errors"
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

func TestIsValidRequest_Error(t *testing.T) {
	isValidRequest(mockTestingT{t: t}, mockRequest{err: errors.New("test error")})
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
	isInvalidRequest(mockTestingT{t: t}, mockRequest{err: errors.New("test error")})
}

func TestIsInvalidRequest_NilError(t *testing.T) {
	isInvalidRequest(mockTestingT{t: t}, mockRequest{err: nil})
}

func TestIsInvalidRequest_WrongValidationError(t *testing.T) {
	err := validation.NewErrRecordIsMissingRequiredField("field")
	isInvalidRequest(mockTestingT{t: t}, mockRequest{err: err})
}

func TestIsInvalidRequest_MockWithValidator(t *testing.T) {
	err := validation.NewErrBadRequestFieldValue("field", "value")
	isInvalidRequest(mockTestingT{t: t}, mockRequest{err: err}, func(t *testing.T, err error) {
		// This should not be called and should be covered by the else branch in isInvalidRequest
	})
}
