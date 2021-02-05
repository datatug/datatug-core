package models

import (
	"encoding/json"
	"fmt"
	"github.com/qri-io/jsonschema"
	"github.com/strongo/validation"
	"time"
)

// Recordset holds data & stats for recordset returned by executed command
type Recordset struct {
	Duration time.Duration     `json:"durationNanoseconds"`
	Columns  []RecordsetColumn `json:"columns"`
	Rows     [][]interface{}   `json:"rows"`
}

// RecordsetColumn describes column in a recordset
type RecordsetColumn struct {
	Name   string          `json:"name"`
	DbType string          `json:"dbType"`
	Meta   *EntityFieldRef `json:"meta"`
}

// RecordsetDefinition describes dataset
type RecordsetDefinition struct {
	ProjectItem   `yaml:"project_item"`
	Tags          []string            `json:"tags,omitempty" yaml:",omitempty"` // consider moving to ProjectItem
	Type          string              `json:"type" yaml:",omitempty"`           // Supported types: "recordset", "json"
	JSONSchema    string              `json:"jsonSchema,omitempty" yaml:",omitempty"`
	Columns       RecordsetColumnDefs `json:"columns,omitempty" yaml:",omitempty"`
	Files         []string            `json:"files,omitempty" yaml:",omitempty"`
	Errors        []string            `json:"errors,omitempty"`
	PrimaryKey    []string            `json:"primaryKey,omitempty"`
	AlternateKeys []AlternateKey      `json:"alternateKey,omitempty"`
}

type AlternateKey struct {
	ID      string
	Columns []string
}

type RecordsetColumnDefs []RecordsetColumnDef

func (v RecordsetColumnDefs) HasColumn(name string) bool {
	for _, c := range v {
		if c.Name == name {
			return true
		}
	}
	return false
}

// Validate returns error if not valid
func (v RecordsetColumnDefs) Validate() error {
	for i, field := range v {
		if err := field.Validate(); err != nil {
			return fmt.Errorf("invalid field at index=%v, name=%v: %w", i, field.Name, err)
		}
	}
	return nil
}

type RecordsetColumnDef struct {
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Required bool            `json:"required,omitempty"`
	Meta     *EntityFieldRef `json:"meta,omitempty"`
}

func (v RecordsetColumnDef) Validate() error {
	return nil
}

// Validate returns error if not valid
func (v RecordsetDefinition) Validate() error {
	if err := v.ProjectItem.Validate(true); err != nil {
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
	if err := v.Columns.Validate(); err != nil {
		return validation.NewErrBadRequestFieldValue("fields", err.Error())
	}

	validateKeyColumnNames := func(field string, columnNames []string) error {
		for i, columnName := range v.PrimaryKey {
			if !v.Columns.HasColumn(columnName) {
				return validation.NewErrBadRecordFieldValue(field, fmt.Sprintf("references unknown column at index=%v", i))
			}
			for j := 0; j < i; j++ {
				if v.Columns[j].Name == columnName {
					return validation.NewErrBadRecordFieldValue(field, fmt.Sprintf("duplicate column name at indexes %v and %v: %v", j, i, columnName))
				}
			}
		}
		return nil
	}
	if err := validateKeyColumnNames("primaryKey", v.PrimaryKey); err != nil {
		return err
	}
	for k, fk := range v.AlternateKeys {
		if err := validateKeyColumnNames(fmt.Sprintf("alternateKeys[%]", k), fk.Columns); err != nil {
			return err
		}
	}
	return nil
}
