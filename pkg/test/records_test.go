package test

import (
	"errors"
	"testing"
)

type mockRecord struct {
	err error
}

func (m mockRecord) Validate() error {
	return m.err
}

type badRecordError struct {
}

func (e badRecordError) Error() string {
	return "bad record"
}

func (e badRecordError) IsValidationError() bool {
	return true
}

func (e badRecordError) IsBadRecordError() bool {
	return true
}

func TestIsValidRecord(t *testing.T) {
	IsValidRecord(t, "valid", mockRecord{err: nil})
}

func TestIsInvalidRecord(t *testing.T) {
	err := badRecordError{}
	IsInvalidRecord(t, "invalid", mockRecord{err: err}, func(t *testing.T, err error) {
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestIsInvalidRecord_NonValidationError(t *testing.T) {
	// We use a subtest to avoid failing the main test
	t.Run("suppress-failure", func(st *testing.T) {
		IsInvalidRecord(st, "non-validation", mockRecord{err: errors.New("test error")})
	})
}

func TestIsInvalidRecord_NilError(t *testing.T) {
	t.Run("suppress-failure", func(st *testing.T) {
		IsInvalidRecord(st, "nil-error", mockRecord{err: nil})
	})
}
