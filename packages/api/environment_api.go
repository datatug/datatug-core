package api

import (
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"github.com/strongo/validation"
)

// GetEnvironmentSummary returns environment summary
func GetEnvironmentSummary(ref dto.ProjectItemRef) (*models.EnvironmentSummary, error) {
	if ref.ProjectID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("projID")
	}
	if ref.ID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("envID")
	}
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err == nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	project := store.Project(ref.ProjectID)
	return project.Environments().Environment(ref.ID).LoadEnvironmentSummary()
}
