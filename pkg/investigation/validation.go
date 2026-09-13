// Package investigation owns the shared Investigation Context value model.
// Transport and incident packages depend on these types rather than defining
// competing representations of the same facts.
package investigation

import (
	"fmt"
	"strings"
)

// ValidationError reports a single contract rule a value violated.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func requireNonEmpty(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return &ValidationError{Field: field, Message: "is required"}
	}
	return nil
}

func requireOneOf(field, value string, allowed ...string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return &ValidationError{Field: field, Message: fmt.Sprintf("must be one of %v, got %q", allowed, value)}
}
