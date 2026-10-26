package walkthrough

import (
	"errors"
	"fmt"
)

// Code is a stable refusal code for a declaration problem. Callers may switch
// on it; human wording is not part of the contract.
type Code string

const (
	// CodeTooLarge means the input exceeds MaxDeclarationBytes.
	CodeTooLarge Code = "too_large"
	// CodeInvalidUTF8 means the input bytes are not valid UTF-8.
	CodeInvalidUTF8 Code = "invalid_utf8"
	// CodeInvalidJSON means the bytes are not one JSON value with JSON syntax.
	CodeInvalidJSON Code = "invalid_json"
	// CodeDuplicateKey means a JSON object repeats a key (including an escaped
	// spelling of the same key).
	CodeDuplicateKey Code = "duplicate_key"
	// CodeTrailingData means non-whitespace bytes follow the JSON value.
	CodeTrailingData Code = "trailing_data"
	// CodeNotObject means the root JSON value is not an object.
	CodeNotObject Code = "not_object"
	// CodeMissingField means a required field is absent.
	CodeMissingField Code = "missing_field"
	// CodeUnknownField means an object carries a field outside its closed set.
	CodeUnknownField Code = "unknown_field"
	// CodeInvalidType means a value has the wrong JSON type (including explicit
	// null where a value is required).
	CodeInvalidType Code = "invalid_type"
	// CodeInvalidValue means a string, enum, integer lexeme or identity does
	// not satisfy its declared rule.
	CodeInvalidValue Code = "invalid_value"
	// CodeTooLong means a text value exceeds its character limit.
	CodeTooLong Code = "too_long"
	// CodeTooMany means an array or total count exceeds its limit.
	CodeTooMany Code = "too_many"
	// CodeDuplicateID means a chapter, former, step or captured identity is
	// repeated in a scope that requires uniqueness.
	CodeDuplicateID Code = "duplicate_id"
	// CodeInvalidReference means a static reference (continuesFrom, chapter
	// exit, readiness/needs pairing) does not resolve.
	CodeInvalidReference Code = "invalid_reference"
	// CodeMissingDefault means a captured value object omits its mandatory
	// literal default.
	CodeMissingDefault Code = "missing_default"
	// CodeUnknownCapture means a captured value names no declared producer.
	CodeUnknownCapture Code = "unknown_capture"
	// CodeCaptureNotEarlier means a captured value references a producer that
	// is not earlier in document order.
	CodeCaptureNotEarlier Code = "capture_not_earlier"
	// CodeCaptureNotAllowed means a capture is attached to a known condition
	// kind that cannot produce a value.
	CodeCaptureNotAllowed Code = "capture_not_allowed"
	// CodeMissingExploreEntry means a chapter has no step available in Explore.
	CodeMissingExploreEntry Code = "missing_explore_entry"
	// CodeInvalidEntryTarget means a chapter's first Explore step targets none.
	CodeInvalidEntryTarget Code = "invalid_entry_target"
	// CodeInvalidPath means an asset path is not a safe project-relative path.
	CodeInvalidPath Code = "invalid_path"
	// CodeMissingContext means a required owner adapter or resolved source
	// context is absent. Nothing may be admitted from a refused context.
	CodeMissingContext Code = "missing_context"
	// CodeNotAdmitted means the trusted owner context does not admit the named
	// package, asset, Case or challenge manifest.
	CodeNotAdmitted Code = "not_admitted"
	// CodeMismatch means two bound identities or references must be equal field
	// for field and are not.
	CodeMismatch Code = "mismatch"
	// CodePinMismatch means a source pin is not the one exact immutable pin its
	// source requires (for example a branch or HEAD where a commit is required).
	CodePinMismatch Code = "pin_mismatch"
	// CodeDigestMismatch means supplied bytes or a repeated digest disagree with
	// the pinned SHA-256.
	CodeDigestMismatch Code = "digest_mismatch"
	// CodeByteCountMismatch means supplied bytes disagree with the pinned byte
	// length.
	CodeByteCountMismatch Code = "byte_count_mismatch"
	// CodeInconsistentSummary means a checkpoint envelope's outcome and
	// whole-attempt completion contradict each other.
	CodeInconsistentSummary Code = "inconsistent_summary"
	// CodeIneligible means a registry entry or namespace may not be used here:
	// a non-public scope or a reference/Test-only result.
	CodeIneligible Code = "ineligible"
)

// Issue is one structured, recoverable declaration problem. Path is a JSON
// Pointer into the input ("" is the root).
type Issue struct {
	Code    Code   `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (i Issue) Error() string {
	return fmt.Sprintf("walkthrough: %s at %q: %s", i.Code, i.Path, i.Message)
}

// Error is the aggregate refusal returned by [Result.Err]. It is recoverable:
// reloading a supported schema or fixing the named declaration fields can
// succeed, and no completion may be recorded from a refused parse.
type Error struct {
	Issues []Issue `json:"issues"`
}

func (e *Error) Error() string {
	if len(e.Issues) == 1 {
		return e.Issues[0].Error()
	}
	return fmt.Sprintf("walkthrough: %d declaration issues: %s", len(e.Issues), e.Issues[0].Error())
}

// Recoverable reports whether retrying with a supported schema or corrected
// bytes can succeed. Every refusal produced by this package is recoverable.
func (e *Error) Recoverable() bool { return true }

// ErrorCode returns the first stable issue code, or "" when err is not a
// walkthrough refusal.
func ErrorCode(err error) Code {
	var refusal *Error
	if errors.As(err, &refusal) && len(refusal.Issues) > 0 {
		return refusal.Issues[0].Code
	}
	var issue Issue
	if errors.As(err, &issue) {
		return issue.Code
	}
	return ""
}

func (c *checker) report(code Code, path, format string, args ...any) {
	c.issues = append(c.issues, Issue{Code: code, Path: path, Message: fmt.Sprintf(format, args...)})
}
