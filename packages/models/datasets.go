package models

import (
	"encoding/json"
	"fmt"
	"github.com/qri-io/jsonschema"
	"github.com/strongo/validation"
)

// DatasetDefinition describes dataset
type DatasetDefinition struct {
	ProjectEntity
	Tags       []string           `json:"tags,omitempty"` // consider moving to ProjectEntity
	Type       string             `json:"type"`           // Supported types: "recordset", "json"
	JSONSchema string             `json:"jsonSchema,omitempty"`
	Fields     RecordsetFieldDefs `json:"fields,omitempty"`
	Files      []string           `json:"files,omitempty"`
}

type RecordsetFieldDefs []RecordsetFieldDef

// Validate returns error if not valid
func (v RecordsetFieldDefs) Validate() error {
	for i, field := range v {
		if err := field.Validate(); err != nil {
			return fmt.Errorf("invalid field at index=%v, name=%v: %w", i, field.Name, err)
		}
	}
	return nil
}

type RecordsetFieldDef struct {
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Required bool            `json:"required"`
	Meta     *EntityFieldRef `json:"meta,omitempty"`
}

func (v RecordsetFieldDef) Validate() error {
	return nil
}

// Validate returns error if not valid
func (v DatasetDefinition) Validate() error {
	if err := v.ProjectEntity.Validate(true); err != nil {
		return err
	}
	switch v.Type {
	case "recordset":
	case "json":
		if v.JSONSchema == "" {
			return validation.NewErrRecordIsMissingRequiredField("jsonSchema")
		}
		schema := &jsonschema.Schema{}
		if err := json.Unmarshal([]byte(v.JSONSchema), schema); err != nil {
			return err
		}
	// OK
	case "":
		return validation.NewErrRecordIsMissingRequiredField("type")
	default:
		return validation.NewErrBadRecordFieldValue("type", "unknown value: "+v.Type)
	}
	if err := v.Fields.Validate(); err != nil {
		return validation.NewErrBadRequestFieldValue("fields", err.Error())
	}
	return nil
}
