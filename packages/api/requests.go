package api

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/validation"
	"strings"
)

// ProjectRef holds store & project parameters
type ProjectRef struct {
	StoreID   string `json:"store"`
	ProjectID string `json:"project"`
}

// Validate returns error if not valid
func (v ProjectRef) Validate() error {
	if v.ProjectID == "" {
		return validation.NewErrRequestIsMissingRequiredField("project")
	}
	if v.StoreID == "" {
		return validation.NewErrRequestIsMissingRequiredField("store")
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

// GetServerDatabasesRequest input for /dbserver/databases API
type GetServerDatabasesRequest struct {
	Project     string `json:"proj"`
	Environment string `json:"env"`
	models.ServerReference
	Credentials *models.Credentials `json:"credentials"`
}

// Validate returns error if not valid
func (v GetServerDatabasesRequest) Validate() error {
	if strings.TrimSpace(v.Project) == "" {
		return validation.NewErrRequestIsMissingRequiredField("proj")
	}
	if strings.TrimSpace(v.Environment) == "" && v.Host == "" {
		return validation.NewErrRequestIsMissingRequiredField("env or host")
	}
	if v.Credentials != nil {
		if err := v.Credentials.Validate(); err != nil {
			return err
		}
	}
	return nil
}
