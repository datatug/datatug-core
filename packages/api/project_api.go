package api

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"github.com/strongo/validation"
)

func validateProjectInput(projectID string) (err error) {
	if projectID == "" {
		return validation.NewErrRequestIsMissingRequiredField("projectID")
	}
	return nil
}

// GetProjects return all projects
func GetProjects(ctx context.Context, storeID string) ([]models.ProjectBrief, error) {
	dal, err := storage.NewDatatugStore(storeID)
	if err != nil {
		return nil, err
	}
	return dal.GetProjects(ctx)
}

// GetProjectSummary returns project summary
func GetProjectSummary(ctx context.Context, ref dto.ProjectRef) (*models.ProjectSummary, error) {
	if ref.ProjectID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("id")
	}
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err != nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	project := store.GetProjectStore(ref.ProjectID)
	projectSummary, err := project.LoadProjectSummary(ctx)
	return &projectSummary, err
}

// CreateProject create a new DataTug project using requested store
func CreateProject(ctx context.Context, request dto.CreateProjectRequest) (*models.ProjectSummary, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	store, err := storage.GetStore(ctx, request.StoreID)
	if err != nil {
		return nil, fmt.Errorf("failed to get store by ID=%v: %w", request.StoreID, err)
	}
	if store == nil {
		return nil, fmt.Errorf("no store returned by storage.GetStore(id=%v)", request.StoreID)
	}
	return store.CreateProject(ctx, request)
}

// GetProjectFull returns full project metadata
func GetProjectFull(ctx context.Context, ref dto.ProjectRef) (*models.DatatugProject, error) {
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err != nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	project := store.GetProjectStore(ref.ProjectID)
	return project.LoadProject(ctx)
}
