package gauth

import (
	"errors"
	"strings"
)

// ServiceAccountDbo represents a Firebase service account reference stored locally.
// It must have a human-readable Name and a Path to the JSON credentials file.
//
// NOTE: This is a data holder and does not load credentials; that is handled
// elsewhere when we actually talk to Firebase APIs.
//
// Validation rules:
// - Name: required, trimmed, non-empty
// - Path: required, trimmed, non-empty
// No file existence check here to allow adding first and validating later in flows.
// This keeps Validate deterministic and unit-testable without filesystem.
//
// Each struct implements Validate() error to follow project conventions.
// (See .junie/CODE_STYLE.md)

type ServiceAccountDbo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Validate implements the interface { Validate() error } required by the project.
func (sa ServiceAccountDbo) Validate() error {
	name := strings.TrimSpace(sa.Name)
	if name == "" {
		return errors.New("service account name is required")
	}
	path := strings.TrimSpace(sa.Path)
	if path == "" {
		return errors.New("service account path is required")
	}
	return nil
}
