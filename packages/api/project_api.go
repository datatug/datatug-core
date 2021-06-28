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

// GetProjects return all projects
func GetProjects(storeID string) ([]models.ProjectBrief, error) {
	dal, err := store.NewDatatugStore(storeID)
	if err != nil {
		return nil, err
	}
	return dal.GetProjects()
}

// GetProjectSummary returns project summary
func GetProjectSummary(ref ProjectRef) (*models.ProjectSummary, error) {
	if ref.ProjectID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("id")
	}
	dal, err := store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return nil, err
	}
	projectSummary, err := dal.LoadProjectSummary(ref.ProjectID)
	return &projectSummary, err
}

// GetProjectFull returns full project metadata
func GetProjectFull(ref ProjectRef) (project *models.DatatugProject, err error) {
	dal, err := store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return
	}
	return dal.LoadProject(ref.ProjectID)
}
