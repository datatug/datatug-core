package models

import (
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/strongo/validation"
)

// CollectionKey defines a key that identifies a table or a view
type CollectionKey struct {
	Name    string `json:"name"`
	Schema  string `json:"schema,omitempty"`
	Catalog string `json:"catalog,omitempty"`
	Ref     *dal.CollectionRef
}

func (v CollectionKey) String() string {
	if v.Ref != nil {
		return v.Ref.String()
	}
	if v.Schema == "" && v.Catalog == "" {
		return v.Name
	}
	if v.Catalog == "" {
		return fmt.Sprintf("%v.%v", v.Schema, v.Name)
	}
	return fmt.Sprintf("%v.%v.%v", v.Catalog, v.Schema, v.Name)
}

// Validate returns error if not valid
func (v CollectionKey) Validate() error {
	if v.Name == "" && v.Ref == nil {
		return validation.NewErrRecordIsMissingRequiredField("name|ref")
	}
	return nil
}
