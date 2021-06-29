package api

import (
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"github.com/strongo/validation"
)

// GetEnvironmentSummary returns environment summary
func GetEnvironmentSummary(ref dto.ProjectItemRef) (envSummary *models.EnvironmentSummary, err error) {
	if ref.ProjectID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("projID")
	}
	if ref.ID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("envID")
	}
	var dal storage.Store
	dal, err = storage.NewDatatugStore(ref.StoreID)
	if err != nil {
		return
	}
	summary, err := dal.LoadEnvironmentSummary(ref.ProjectID, ref.ID)
	return &summary, err
}
