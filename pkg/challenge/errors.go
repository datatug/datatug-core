package challenge

import (
	"errors"
	"fmt"
)

// Code is a stable refusal code for a definition or answer problem.
type Code string

const (
	// CodeInvalidManifest covers structural, range and reference violations.
	CodeInvalidManifest Code = "invalid_manifest"
	// CodeUnsupportedSchema refuses a schema version this package cannot grade.
	CodeUnsupportedSchema Code = "unsupported_schema"
	// CodeUnsupportedGradingVersion refuses changed predicate semantics.
	CodeUnsupportedGradingVersion Code = "unsupported_grading_version"
	// CodeUnsupportedDialect refuses a dialect the browser runtime cannot host.
	CodeUnsupportedDialect Code = "unsupported_dialect"
	// CodeUnsupportedEngine refuses a revision pinned to another engine.
	CodeUnsupportedEngine Code = "unsupported_engine"
	// CodeFixtureMissing refuses a pin without bytes.
	CodeFixtureMissing Code = "fixture_missing"
	// CodeFixtureUnknown refuses bytes that no revision pins.
	CodeFixtureUnknown Code = "fixture_unknown"
	// CodeFixtureBytes refuses a byte length that differs from its pin.
	CodeFixtureBytes Code = "fixture_bytes"
	// CodeFixtureHash refuses bytes whose SHA-256 differs from the pin.
	CodeFixtureHash Code = "fixture_hash"
	// CodeInvalidAnswer covers an answer that cannot be graded at all.
	CodeInvalidAnswer Code = "invalid_answer"
)

// Error is a definition or answer refusal. Definition refusals are
// recoverable: reloading a supported revision or its original bytes resolves
// them, and no completion may be recorded from a refused open.
type Error struct {
	Code    Code   `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("challenge: %s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("challenge: %s: %s: %s", e.Code, e.Field, e.Message)
}

// Recoverable reports whether retrying with a supported revision or the
// pinned bytes can succeed. Every refusal produced here is recoverable.
func (e *Error) Recoverable() bool { return true }

// ErrorCode returns the stable refusal code, or "" for an unclassified error.
func ErrorCode(err error) Code {
	var refusal *Error
	if errors.As(err, &refusal) {
		return refusal.Code
	}
	return ""
}

func refuse(code Code, field, format string, args ...any) *Error {
	return &Error{Code: code, Field: field, Message: fmt.Sprintf(format, args...)}
}
