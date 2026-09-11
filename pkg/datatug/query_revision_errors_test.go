package datatug

import (
	"errors"
	"fmt"
	"testing"
)

func TestInvalidQueryLocationError(t *testing.T) {
	err := &InvalidQueryLocationError{FolderPath: "a/../b", ID: "q1", Reason: "escapes queries root"}
	if !IsInvalidQueryLocation(err) {
		t.Fatal("expected IsInvalidQueryLocation to be true")
	}
	if !errors.Is(err, ErrInvalidQueryLocation) {
		t.Fatal("expected errors.Is(err, ErrInvalidQueryLocation) to be true")
	}
	wrapped := fmt.Errorf("wrapped: %w", err)
	if !IsInvalidQueryLocation(wrapped) {
		t.Fatal("expected IsInvalidQueryLocation to see through fmt.Errorf wrapping")
	}
	if IsIncompleteQueryRecord(err) || IsQueryRevisionConflict(err) {
		t.Fatal("expected the other two predicates to be false")
	}
	if err.Error() == "" {
		t.Fatal("expected a non-empty error message")
	}
}

func TestIncompleteQueryRecordError(t *testing.T) {
	err := &IncompleteQueryRecordError{FolderPath: "folder1", ID: "q1", Reason: "body sidecar missing"}
	if !IsIncompleteQueryRecord(err) {
		t.Fatal("expected IsIncompleteQueryRecord to be true")
	}
	if !errors.Is(err, ErrIncompleteQueryRecord) {
		t.Fatal("expected errors.Is(err, ErrIncompleteQueryRecord) to be true")
	}
	if IsInvalidQueryLocation(err) || IsQueryRevisionConflict(err) {
		t.Fatal("expected the other two predicates to be false")
	}
}

func TestQueryRevisionConflictError(t *testing.T) {
	err := &QueryRevisionConflictError{
		FolderPath: "folder1", ID: "q1",
		Expected: "rev-1", Actual: "rev-2", Reason: "stale revision",
	}
	if !IsQueryRevisionConflict(err) {
		t.Fatal("expected IsQueryRevisionConflict to be true")
	}
	if !errors.Is(err, ErrQueryRevisionConflict) {
		t.Fatal("expected errors.Is(err, ErrQueryRevisionConflict) to be true")
	}
	if IsInvalidQueryLocation(err) || IsIncompleteQueryRecord(err) {
		t.Fatal("expected the other two predicates to be false")
	}
	var asErr *QueryRevisionConflictError
	if !errors.As(err, &asErr) {
		t.Fatal("expected errors.As to recover the concrete type")
	}
	if asErr.Expected != "rev-1" || asErr.Actual != "rev-2" {
		t.Fatalf("expected fields to survive errors.As, got: %+v", asErr)
	}
}
