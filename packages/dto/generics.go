package dto

import "github.com/strongo/validation"

// ProjectRef holds storage & project parameters
type ProjectRef struct {
	StoreID   string `json:"storage"`
	ProjectID string `json:"project"`
}

// Validate returns error if not valid
func (v ProjectRef) Validate() error {
	if v.ProjectID == "" {
		return validation.NewErrRequestIsMissingRequiredField("project")
	}
	if v.StoreID == "" {
		return validation.NewErrRequestIsMissingRequiredField("storage")
	}
	return nil

}

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
