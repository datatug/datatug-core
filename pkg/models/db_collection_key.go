package models

import (
	"fmt"

	"github.com/strongo/validation"
)

type CollectionKey struct {
}

// TableKey defines a key that identifies a table or a view
type TableKey struct {
	Name    string `json:"name"`
	Schema  string `json:"schema,omitempty"`
	Catalog string `json:"catalog,omitempty"`
}

func (v TableKey) String() string {
	if v.Schema == "" && v.Catalog == "" {
		return v.Name
	}
	if v.Catalog == "" {
		return fmt.Sprintf("%v.%v", v.Schema, v.Name)
	}
	return fmt.Sprintf("%v.%v.%v", v.Catalog, v.Schema, v.Name)
}

// Validate returns error if not valid
func (v TableKey) Validate() error {
	if v.Name == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	return nil
}
