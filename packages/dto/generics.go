package dto

import "github.com/strongo/validation"

// ProjectItemRef holds ProjectRef & ID parameters
type ProjectItemRef struct {
	ProjectRef
	ID string
}

// Validate returns error if not valid
func (v ProjectItemRef) Validate() error {
	if err := v.ProjectRef.Validate(); err != nil {
		return err
	}
	if v.ID == "" {
		return validation.NewErrRequestIsMissingRequiredField("id")
	}
	return nil
}

// CreateFolder defines request for folder creation
type CreateFolder struct {
	ProjectRef
	Name string `json:"name"`
	Path string `json:"path"`
	Note string `json:"note,omitempty"`
}

// Validate returns error if not valid
func (v CreateFolder) Validate() error {
	if err := v.ProjectRef.Validate(); err != nil {
		return nil
	}
	if v.Name == "" {
		return validation.NewErrRequestIsMissingRequiredField("name")
	}
	return nil

}
