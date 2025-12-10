package datatug

import (
	"fmt"

	"github.com/strongo/validation"
)

// UserDatatugInfo holds user info for DataTug project
type UserDatatugInfo struct {
	Stores map[string]DatatugStoreBrief `json:"stores,omitempty" firestore:"stores,omitempty"`
}

// DatatugStoreBrief stores info about datatug storage known to a user
type DatatugStoreBrief struct {
	Type     string                  `json:"type,omitempty" firestore:"type,omitempty" yaml:"type,omitempty"`
	Title    string                  `json:"title,omitempty" firestore:"title,omitempty" yaml:"title,omitempty"`
	Projects map[string]ProjectBrief `json:"projects,omitempty" firestore:"projects,omitempty"`
}

// Validate returns error if not valid
func (v DatatugStoreBrief) Validate() error {
	if v.Type == "" {
		return validation.NewErrRecordIsMissingRequiredField("type")
	}
	if !IsValidateStoreType(v.Type) {
		return validation.NewErrBadRecordFieldValue("type", "unsupported value: "+v.Type)
	}
	for i, p := range v.Projects {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("invalid project at index %v: %w", i, err)
		}
	}
	return nil
}

var validStoreTypes = []string{"firestore", "github.com", "agent"}

// IsValidateStoreType checks if storage type has valid value
func IsValidateStoreType(v string) bool {
	if v == "" {
		return false
	}
	for _, s := range validStoreTypes {
		if s == v {
			return true
		}
	}
	return false
}

// Validate returns error if not valid
func (v UserDatatugInfo) Validate() error {
	if len(v.Stores) > 0 {
		for storeID, storeBrief := range v.Stores {
			if err := storeBrief.Validate(); err != nil {
				return validation.NewErrBadRecordFieldValue(fmt.Sprintf("stores[%v]", storeID), err.Error())
			}

		}
	}
	return nil
}

// DatatugUser defines a user record with props related to Datatug
type DatatugUser struct {
	Datatug *UserDatatugInfo `json:"datatug,omitempty" firestore:"datatug,omitempty"`
}

// Validate returns error if not valid
func (v DatatugUser) Validate() error {
	if v.Datatug != nil {
		if err := v.Datatug.Validate(); err != nil {
			return fmt.Errorf("invalid datatug property: %w", err)
		}
	}
	return nil
}
