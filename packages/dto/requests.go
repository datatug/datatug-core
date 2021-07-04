package dto

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/validation"
	"strings"
)

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

type CreateQuery struct {
	ProjectRef
	Folder string          `json:"folder"`
	Query  models.QueryDef `json:"query"`
}

type UpdateQuery struct {
	ProjectItemRef
	Query models.QueryDef `json:"query"`
}
