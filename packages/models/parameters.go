package models

import (
	"fmt"
	"github.com/strongo/validation"
	"reflect"
)

// Parameter defines input parameter for a board, widget, etc.
type Parameter struct {
	Name         string           `json:"name"`
	Type         string           `json:"type"`
	DefaultValue interface{}      `json:"defaultValue"`
	Title        string           `json:"title,omitempty"`
	IsRequired   bool             `json:"isRequired,omitempty"`
	IsMultiValue bool             `json:"isMultiValue,omitempty"`
	MaxLength    int              `json:"maxLength,omitempty"`
	MinLength    int              `json:"minLength,omitempty"`
	Meta         *EntityFieldRef  `json:"meta,omitempty"`
	Lookup       *ParameterLookup `json:"lookup,omitempty"`
}

// Validate returns error if failed
func (v Parameter) Validate() error {
	if v.Name == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	if v.Type == "" {
		return validation.NewErrRecordIsMissingRequiredField("type")
	}
	if v.DefaultValue != nil {
		ok := true
		switch v.Type {
		case "string":
			_, ok = v.DefaultValue.(string)
		case "integer":
			_, ok = v.DefaultValue.(int)
		case "number":
			_, ok = v.DefaultValue.(float64)
		case "boolean":
			_, ok = v.DefaultValue.(bool)
		case "bit":
			_, ok = v.DefaultValue.(int)
		}
		if !ok {
			return validation.NewErrBadRecordFieldValue("defaultValue",
				fmt.Sprintf("actual type %v does not match expected type %v",
					reflect.TypeOf(v.DefaultValue).Name(), v.Type))
		}
	}
	return nil
}

// Parameters slice of `Parameter`
type Parameters []Parameter

// Validate returns error if failed
func (v Parameters) Validate() error {
	for i, p := range v {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("invalid parameter at index %v: %w", i, err)
		}
	}
	return nil
}
