package apicontract

import "testing"

func TestValidationError_Error(t *testing.T) {
	withField := &ValidationError{Field: "project", Message: "is required"}
	if got, want := withField.Error(), "project: is required"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	withoutField := &ValidationError{Message: "something went wrong"}
	if got, want := withoutField.Error(), "something went wrong"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
