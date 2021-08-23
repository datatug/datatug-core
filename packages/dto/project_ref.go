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
