package test

import (
	"errors"
	"testing"
)

type mockRequest struct {
	err error
}

func (m mockRequest) Validate() error {
	return m.err
}

type badRequestError struct {
}

func (e badRequestError) Error() string {
	return "bad request"
}

func (e badRequestError) IsValidationError() bool {
	return true
}

func (e badRequestError) IsBadRequestError() bool {
	return true
}

func TestIsValidRequest(t *testing.T) {
	IsValidRequest(t, "valid", mockRequest{err: nil})
}

func TestIsInvalidRequest(t *testing.T) {
	err := badRequestError{}
	IsInvalidRequest(t, "invalid", mockRequest{err: err}, func(t *testing.T, err error) {
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestIsInvalidRequest_NonValidationError(t *testing.T) {
	t.Run("suppress-failure", func(st *testing.T) {
		IsInvalidRequest(st, "non-validation", mockRequest{err: errors.New("test error")})
	})
}

func TestIsInvalidRequest_NilError(t *testing.T) {
	t.Run("suppress-failure", func(st *testing.T) {
		IsInvalidRequest(st, "nil-error", mockRequest{err: nil})
	})
}
