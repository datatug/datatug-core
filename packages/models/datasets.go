package models

import (
	"encoding/json"
	"github.com/qri-io/jsonschema"
	"github.com/strongo/validation"
)

// DatasetDefinition describes dataset
type DatasetDefinition struct {
	ProjectEntity
	Type       string `json:"type"`
	JSONSchema string `json:"jsonSchema"`
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
	return nil
}
