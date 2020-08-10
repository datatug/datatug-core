package api

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
)

func validateProjectInput(projectID string) (err error) {
	if projectID == "" {
		return validation.NewErrRequestIsMissingRequiredField("projectID")
	}
	return nil
}

// GetProjectSummary returns project summary
func GetProjectSummary(id string) (*models.ProjectSummary, error) {
	if id == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("id")
	}
	projectSummary, err := store.Current.GetProjectSummary(id)
	return &projectSummary, err
}

// GetProjectFull returns full project metadata
func GetProjectFull(id string) (project *models.DataTugProject, err error) {
	return store.Current.GetProject(id)
}
