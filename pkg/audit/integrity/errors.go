package integrity

import (
	"errors"
	"fmt"
)

// Sentinel errors. Each tamper class has its own sentinel; a [*VerifyError]
// matches it with errors.Is.
var (
	// ErrInvalidArgument reports a caller error (nil input, empty id).
	ErrInvalidArgument = errors.New("audit integrity: invalid argument")
	// ErrInvalidWindow reports a malformed window identity.
	ErrInvalidWindow = errors.New("audit integrity: invalid window")
	// ErrInvalidStatement reports a statement that fails its own rules.
	ErrInvalidStatement = errors.New("audit integrity: invalid statement")
	// ErrMalformedHash reports a hash that is not 64 hex characters.
	ErrMalformedHash = errors.New("audit integrity: malformed hash")

	// ErrUnsupportedVersion reports an event encoding version this package does not know.
	ErrUnsupportedVersion = errors.New("audit integrity: unsupported encoding version")
	// ErrMalformedEvent reports a disclosed event whose field set, order or
	// value encoding is not canonical.
	ErrMalformedEvent = errors.New("audit integrity: malformed event")
	// ErrUnknownField reports a field name outside the schema.
	ErrUnknownField = errors.New("audit integrity: unknown field")

	// ErrBadSignature reports a statement whose signature does not verify
	// (tampered statement, or a root re-signed with another key).
	ErrBadSignature = errors.New("audit integrity: bad signature")
	// ErrUnknownKey reports a signature by a key the verifier does not trust.
	ErrUnknownKey = errors.New("audit integrity: unknown signing key")
	// ErrFieldTampered reports a disclosed field whose value does not match its commitment.
	ErrFieldTampered = errors.New("audit integrity: field does not match its commitment")
	// ErrInclusionProof reports an event whose leaf is not in the window root.
	ErrInclusionProof = errors.New("audit integrity: inclusion proof does not verify")
	// ErrLeafReordered reports an event that is authentic but sits at another
	// position than the one it claims.
	ErrLeafReordered = errors.New("audit integrity: leaf reordered")
	// ErrLeafOrder reports events not listed in strictly ascending insertion order.
	ErrLeafOrder = errors.New("audit integrity: events not in insertion order")
	// ErrLeafIndex reports an event index outside its window.
	ErrLeafIndex = errors.New("audit integrity: leaf index out of range")
	// ErrIncompleteWindow reports a window exported as whole that lacks leaves
	// (for example a truncated window).
	ErrIncompleteWindow = errors.New("audit integrity: whole window is missing events")
	// ErrChainBroken reports a window whose previous-root link does not match.
	ErrChainBroken = errors.New("audit integrity: chain broken")
	// ErrMissingWindow reports a window missing from a claimed-contiguous range.
	ErrMissingWindow = errors.New("audit integrity: window missing from range")
	// ErrWindowSequence reports windows out of order, duplicated, outside the
	// range, or for another Space or project.
	ErrWindowSequence = errors.New("audit integrity: window sequence invalid")
	// ErrMissingSeal reports a window entry with neither a signed root nor a tombstone.
	ErrMissingSeal = errors.New("audit integrity: window has no signed root or tombstone")
	// ErrAnchorMismatch reports a root that differs from an independently anchored copy.
	ErrAnchorMismatch = errors.New("audit integrity: root differs from anchored root")
	// ErrStatementMismatch reports two signed statements that disagree about one window.
	ErrStatementMismatch = errors.New("audit integrity: statements disagree")
	// ErrForgedTombstone reports a tombstone that does not stand: bad
	// signature, disagreeing root, deletion before expiry or under a legal
	// hold, or events still present. The cause is attached.
	ErrForgedTombstone = errors.New("audit integrity: tombstone does not verify")
	// ErrTombstoneBeforeExpiry reports a tombstone dated before the window expired.
	ErrTombstoneBeforeExpiry = errors.New("audit integrity: deleted before expiry")
	// ErrDeletedUnderHold reports a tombstone dated while a legal hold covered the window.
	ErrDeletedUnderHold = errors.New("audit integrity: deleted under legal hold")
	// ErrTombstoneHasEvents reports a tombstoned window exported with events.
	ErrTombstoneHasEvents = errors.New("audit integrity: tombstoned window has events")
	// ErrHoldReleaseNeedsTwo reports a hold release that names fewer than two distinct people.
	ErrHoldReleaseNeedsTwo = errors.New("audit integrity: hold release needs two people")
	// ErrIdentityLeak reports an erased user id still present in an export.
	ErrIdentityLeak = errors.New("audit integrity: erased identity present")
	// ErrPseudonymCollision reports repeated pseudonym collisions (a broken random source).
	ErrPseudonymCollision = errors.New("audit integrity: pseudonym collision")
)

// FieldError names the event field a failure belongs to.
type FieldError struct {
	Field string
	Err   error
}

func (e *FieldError) Error() string { return fmt.Sprintf("field %q: %v", e.Field, e.Err) }

// Unwrap returns the underlying error.
func (e *FieldError) Unwrap() error { return e.Err }

// VerifyError is the error [VerifyExport] returns. It names the first broken
// window and, where it applies, the event (position in the export window) and
// field.
type VerifyError struct {
	// Window is the first broken window.
	Window WindowID
	// Event is the position of the failing event in the export window, or -1.
	Event int
	// Field is the failing field, if any.
	Field string
	// Err is the tamper-class sentinel.
	Err error
	// Cause is the underlying failure, if different from Err.
	Cause error
}

func (e *VerifyError) Error() string {
	s := fmt.Sprintf("window %s: %v", e.Window.Key(), e.Err)
	if e.Event >= 0 {
		s += fmt.Sprintf(" (event %d)", e.Event)
	}
	if e.Field != "" {
		s += fmt.Sprintf(" (field %q)", e.Field)
	}
	if e.Cause != nil {
		s += ": " + e.Cause.Error()
	}
	return s
}

// Unwrap exposes both the sentinel and the cause to errors.Is and errors.As.
func (e *VerifyError) Unwrap() []error {
	if e.Cause == nil {
		return []error{e.Err}
	}
	return []error{e.Err, e.Cause}
}
