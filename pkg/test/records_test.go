package test

import (
	"errors"
	"testing"

	"github.com/strongo/validation"
)

type mockTestingT struct {
	t *testing.T
}

func (m mockTestingT) Helper() {}
func (m mockTestingT) Run(name string, f func(t *testing.T)) bool {
	return m.t.Run(name, f)
}
func (m mockTestingT) Error(args ...interface{}) {
	_ = args
	//m.t.Log("Expected error (mock):", args)
}
func (m mockTestingT) Errorf(format string, args ...interface{}) {
	_ = format
	_ = args
	//m.t.Logf("Expected error (mock): "+format, args...)
}

type mockRecord struct {
	err error
}

func (m mockRecord) Validate() error {
	return m.err
}

func TestIsValidRecord(t *testing.T) {
	IsValidRecord(t, "valid", mockRecord{err: nil})
}

func TestIsValidRecord_Error(t *testing.T) {
	isValidRecord(mockTestingT{t: t}, mockRecord{err: errors.New("test error")})
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
	isInvalidRecord(mockTestingT{t: t}, mockRecord{err: errors.New("test error")})
}

func TestIsInvalidRecord_NilError(t *testing.T) {
	isInvalidRecord(mockTestingT{t: t}, mockRecord{err: nil})
}

func TestIsInvalidRecord_WrongValidationError(t *testing.T) {
	err := validation.NewErrBadRequestFieldValue("field", "value")
	isInvalidRecord(mockTestingT{t: t}, mockRecord{err: err})
}

func TestIsInvalidRecord_MockWithValidator(t *testing.T) {
	err := validation.NewErrRecordIsMissingRequiredField("field")
	isInvalidRecord(mockTestingT{t: t}, mockRecord{err: err}, func(t *testing.T, err error) {
		// This should not be called and should be covered by the else branch in isInvalidRecord
	})
}
