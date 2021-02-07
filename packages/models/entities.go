package models

import (
	"fmt"
	"github.com/datatug/datatug/packages/slice"
	"github.com/strongo/validation"
	"regexp"
	"strings"
)

// Entities is a slice of *Entity
type Entities []*Entity

// GetEntityByID return an entity by ID
func (v Entities) GetEntityByID(id string) (entity *Entity) {
	for _, entity = range v {
		if entity.ID == id {
			return
		}
	}
	return nil
}

// Validate returns error if failed
func (v Entities) Validate() error {
	for i, item := range v {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("validation failed for entity at index=%v, id=%v: %w", i, item.ID, err)
		}
	}
	return nil
}

// IDs returns slice of IDs of db models
func (v Entities) IDs() (ids []string) {
	if len(v) == 0 {
		return
	}
	ids = make([]string, len(v))
	for i, item := range v {
		ids[i] = item.ID
	}
	return
}

// ProjEntityBrief hold brief info about entity in project file
type ProjEntityBrief struct {
	ProjectItem
}

// Entity hold full info about entity
type Entity struct {
	ProjEntityBrief
	Fields EntityFields `json:"fields,omitempty" firestore:"fields,omitempty"`
	Tables TableKeys    `json:"tables,omitempty" firestore:"tables,omitempty"`
}

// Validate returns error if not valid
func (v Entity) Validate() error {
	if err := v.ProjEntityBrief.Validate(false); err != nil {
		return err
	}
	if err := v.Fields.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("fields", err.Error())
	}
	if err := v.Tables.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("tables", err.Error())
	}
	return nil
}

// NamePattern hold patterns for names
type NamePattern struct {
	Regexp   string `json:"regexp,omitempty"`
	Wildcard string `json:"wildcard,omitempty"`
}

// Validate returns error if not valid
func (v NamePattern) Validate() error {
	if v.Regexp != "" && v.Wildcard != "" {
		return validation.NewErrBadRecordFieldValue("regexp&wildcard", "only 1 of pattern fields has to be set")
	}
	if v.Regexp == "" && v.Wildcard == "" {
		return validation.NewErrRecordIsMissingRequiredField("regexp&wildcard")
	}
	if v.Regexp != "" {
		if _, err := regexp.Compile(v.Regexp); err != nil {
			return validation.NewErrBadRecordFieldValue("regexp", err.Error())
		}
	}
	return nil
}

// EntityField hold info about entity field
type EntityField struct {
	ID          string       `json:"id" firestore:"id"`
	Type        string       `json:"type" firestore:"type"`
	Title       string       `json:"title,omitempty" firestore:"title,omitempty"`
	IsKeyField  bool         `json:"isKeyField,omitempty" firestore:"isKeyField,omitempty"`
	NamePattern *NamePattern `json:"namePattern" firestore:"namePattern"`
}

// Validate returns error if not valid
func (v EntityField) Validate() error {
	if v.ID == "" {
		return validation.NewErrRequestIsMissingRequiredField("id")
	}
	if v.Type == "" {
		return validation.NewErrRequestIsMissingRequiredField("type")
	}
	if slice.IndexOfString(KnownTypes, v.Type) < 0 {
		return validation.NewErrBadRecordFieldValue("type", fmt.Sprintf("unknown field type: %v: expected one of: %v", v.Type, strings.Join(KnownTypes, ", ")))
	}
	if err := v.NamePattern.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("namePattern", err.Error())
	}
	return nil
}

// EntityFields is a slice of EntityField
type EntityFields []EntityField

// Validate returns error if not valid
func (v EntityFields) Validate() error {
	for i, f := range v {
		if err := f.Validate(); err != nil {
			return fmt.Errorf("invalid fields[%v]: %w", i, err)
		}
	}
	return nil
}

// EntityFieldRef holds reference to entity field
type EntityFieldRef struct {
	Entity string `json:"entity"`
	Field  string `json:"field"`
}

// Validate returns error if not valid
func (v EntityFieldRef) Validate() error {
	if strings.TrimSpace(v.Entity) == "" {
		return validation.NewErrRecordIsMissingRequiredField("entity")
	}
	if strings.TrimSpace(v.Field) == "" {
		return validation.NewErrRecordIsMissingRequiredField("field")
	}
	return nil
}

