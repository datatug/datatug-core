package datatug

import (
	"fmt"

	"github.com/strongo/validation"
)

// Action does something that affects context
type Action struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
	Next Actions     `json:"next"`
}

// Validate returns error if not valid
func (v Action) Validate() error {
	switch v.Type {
	case "":
		return validation.NewErrRecordIsMissingRequiredField("type")
	case "sql", "http":
	default:
		return validation.NewErrBadRecordFieldValue("type", "unsupported type: "+v.Type)
	}
	return nil
}

// Actions is slice of `Action`
type Actions []Action

// Validate returns error if not valid
func (v Actions) Validate() error {
	for i, a := range v {
		if err := a.Validate(); err != nil {
			return fmt.Errorf("invalid action at index %v: %w", i, err)
		}
	}
	return nil
}
