package filestore

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/strongo/validation"
)

// Task 1: PutQuery requires exactly one of IfNoneMatch/IfMatch and must
// reject an invalid combination before any I/O - validateQueryWriteCondition
// is the pure check PutQuery runs first.

func TestValidateQueryWriteCondition_NeitherSetIsInvalid(t *testing.T) {
	err := validateQueryWriteCondition(datatug.QueryWriteCondition{})
	if err == nil {
		t.Fatal("expected an error when neither IfNoneMatch nor IfMatch is set")
	}
	if !validation.IsBadRequestError(err) {
		t.Fatalf("expected a bad-request validation error, got %T: %v", err, err)
	}
}

func TestValidateQueryWriteCondition_BothSetIsInvalid(t *testing.T) {
	err := validateQueryWriteCondition(datatug.QueryWriteCondition{IfNoneMatch: true, IfMatch: "rev-1"})
	if err == nil {
		t.Fatal("expected an error when both IfNoneMatch and IfMatch are set")
	}
	if !validation.IsBadRequestError(err) {
		t.Fatalf("expected a bad-request validation error, got %T: %v", err, err)
	}
}

func TestValidateQueryWriteCondition_IfNoneMatchOnlyIsValid(t *testing.T) {
	if err := validateQueryWriteCondition(datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("expected IfNoneMatch alone to be valid, got: %v", err)
	}
}

func TestValidateQueryWriteCondition_IfMatchOnlyIsValid(t *testing.T) {
	if err := validateQueryWriteCondition(datatug.QueryWriteCondition{IfMatch: "rev-1"}); err != nil {
		t.Fatalf("expected IfMatch alone to be valid, got: %v", err)
	}
}

func TestValidateQueryWriteCondition_IfMatchEmptyStringIsInvalid(t *testing.T) {
	// IfMatch == "" with IfNoneMatch == false is indistinguishable from the
	// zero value, which must not silently mean "match anything".
	err := validateQueryWriteCondition(datatug.QueryWriteCondition{IfMatch: ""})
	if err == nil {
		t.Fatal("expected an error for the zero-value condition")
	}
}
