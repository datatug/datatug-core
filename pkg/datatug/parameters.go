package datatug

import (
	"fmt"
	"reflect"

	"github.com/strongo/validation"
)

// ParameterDef defines input parameter for a board, widget, etc.
type ParameterDef struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"`
	Title        string           `json:"title,omitempty"`
	DefaultValue interface{}      `json:"defaultValue,omitempty"`
	IsRequired   bool             `json:"isRequired,omitempty"`
	IsMultiValue bool             `json:"isMultiValue,omitempty"`
	MaxLength    int              `json:"maxLength,omitempty"`
	MinLength    int              `json:"minLength,omitempty"`
	Meta         *EntityFieldRef  `json:"meta,omitempty"`
	Lookup       *ParameterLookup `json:"lookup,omitempty"`
}

// Validate returns error if failed
func (v ParameterDef) Validate() error {
	if v.ID == "" {
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

// Parameters slice of `ParameterDef`
type Parameters []ParameterDef

// Validate returns error if failed
func (v Parameters) Validate() error {
	for i, p := range v {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("invalid parameter at index %v: %w", i, err)
		}
	}
	return nil
}

// Parameter defines parameter
type Parameter struct {
	ID    string      `json:"id"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}
