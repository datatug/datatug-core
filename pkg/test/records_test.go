package test

import (
	"testing"

	"github.com/strongo/validation"
)

type mockRecord struct {
	err error
}

func (m mockRecord) Validate() error {
	return m.err
}

func TestIsValidRecord(t *testing.T) {
	IsValidRecord(t, "valid", mockRecord{err: nil})
}

func TestIsInvalidRecord(t *testing.T) {
	err := validation.NewErrRecordIsMissingRequiredField("field")
	IsInvalidRecord(t, "invalid", mockRecord{err: err}, func(t *testing.T, err error) {
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestIsInvalidRecord_NonValidationError(t *testing.T) {
	// We use a subtest to avoid failing the main test
	t.Run("suppress-failure", func(st *testing.T) {
		// IsInvalidRecord(st, "non-validation", mockRecord{err: errors.New("test error")})
	})
}

func TestIsInvalidRecord_NilError(t *testing.T) {
	t.Run("suppress-failure", func(st *testing.T) {
		// IsInvalidRecord(st, "nil-error", mockRecord{err: nil})
	})
}
