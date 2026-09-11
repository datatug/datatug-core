package datatug

import (
	"errors"
	"fmt"
)

// ErrInvalidQueryLocation is the sentinel every InvalidQueryLocationError
// wraps. A later HTTP adapter maps it to 400.
var ErrInvalidQueryLocation = errors.New("invalid query location")

// InvalidQueryLocationError reports that a query's folder path or ID failed
// the safety checks a RevisionedQueriesStore write enforces before any I/O:
// an absolute path, a path-traversal or hidden ("."-prefixed) segment, a
// path separator or NUL byte inside a single ID/folder segment, an attempt
// to escape the project's queries root, or a symlink/non-directory found
// while resolving the location.
type InvalidQueryLocationError struct {
	FolderPath string
	ID         string
	Reason     string
}

func (e *InvalidQueryLocationError) Error() string {
	return fmt.Sprintf("invalid query location (folder=%q, id=%q): %s", e.FolderPath, e.ID, e.Reason)
}

// Unwrap makes errors.Is(err, ErrInvalidQueryLocation) true for every
// *InvalidQueryLocationError.
func (e *InvalidQueryLocationError) Unwrap() error {
	return ErrInvalidQueryLocation
}

// IsInvalidQueryLocation reports whether err is (or wraps) an
// InvalidQueryLocationError.
func IsInvalidQueryLocation(err error) bool {
	return errors.Is(err, ErrInvalidQueryLocation)
}

// ErrIncompleteQueryRecord is the sentinel every IncompleteQueryRecordError
// wraps. A later HTTP adapter maps it to 400 (a malformed/incomplete
// resource state) rather than treating it as "not found."
var ErrIncompleteQueryRecord = errors.New("incomplete query record")

// IncompleteQueryRecordError reports that RevisionedQueriesStore.LoadQueryRevision
// found only part of a query's committed pair - the JSON metadata file
// without its expected body sidecar, or a body sidecar without its JSON
// metadata file. It is returned only by the strict revisioned load; the
// tolerant QueriesStore.LoadQuery keeps treating a missing body sidecar as
// an empty body.
type IncompleteQueryRecordError struct {
	FolderPath string
	ID         string
	Reason     string
}

func (e *IncompleteQueryRecordError) Error() string {
	return fmt.Sprintf("incomplete query record (folder=%q, id=%q): %s", e.FolderPath, e.ID, e.Reason)
}

// Unwrap makes errors.Is(err, ErrIncompleteQueryRecord) true for every
// *IncompleteQueryRecordError.
func (e *IncompleteQueryRecordError) Unwrap() error {
	return ErrIncompleteQueryRecord
}

// IsIncompleteQueryRecord reports whether err is (or wraps) an
// IncompleteQueryRecordError.
func IsIncompleteQueryRecord(err error) bool {
	return errors.Is(err, ErrIncompleteQueryRecord)
}

// ErrQueryRevisionConflict is the sentinel every QueryRevisionConflictError
// wraps. A later HTTP adapter maps it to 409.
var ErrQueryRevisionConflict = errors.New("query revision conflict")

// QueryRevisionConflictError reports that a RevisionedQueriesStore.PutQuery
// or DeleteQueryRevision precondition was not met: an IfNoneMatch create
// found an existing record, or an IfMatch/expected revision did not equal
// the record's current revision (including because the record does not
// exist, reported with Actual == ""). No persisted file was changed.
type QueryRevisionConflictError struct {
	FolderPath string
	ID         string
	Expected   QueryRevision // the revision the caller supplied, "" for an IfNoneMatch create
	Actual     QueryRevision // the record's current revision, "" if it does not exist
	Reason     string
}

func (e *QueryRevisionConflictError) Error() string {
	return fmt.Sprintf("query revision conflict (folder=%q, id=%q): %s", e.FolderPath, e.ID, e.Reason)
}

// Unwrap makes errors.Is(err, ErrQueryRevisionConflict) true for every
// *QueryRevisionConflictError.
func (e *QueryRevisionConflictError) Unwrap() error {
	return ErrQueryRevisionConflict
}

// IsQueryRevisionConflict reports whether err is (or wraps) a
// QueryRevisionConflictError.
func IsQueryRevisionConflict(err error) bool {
	return errors.Is(err, ErrQueryRevisionConflict)
}
