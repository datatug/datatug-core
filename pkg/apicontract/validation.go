package apicontract

import (
	"fmt"
	"strings"

	"github.com/datatug/datatug-core/pkg/investigation"
)

// requireNonEmpty returns a *ValidationError naming field when value is
// empty or all whitespace.
func requireNonEmpty(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return &ValidationError{Field: field, Message: "is required"}
	}
	return nil
}

// requireOneOf returns a *ValidationError naming field when value is not one
// of allowed.
func requireOneOf(field, value string, allowed ...string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return &ValidationError{Field: field, Message: fmt.Sprintf("must be one of %v, got %q", allowed, value)}
}

// ValidationError remains an apicontract alias for compatibility.
type ValidationError = investigation.ValidationError
