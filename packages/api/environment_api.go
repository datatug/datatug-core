package api

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
)

// GetEnvironmentSummary returns environment summary
func GetEnvironmentSummary(ref ProjectItemRef) (envSummary *models.EnvironmentSummary, err error) {
	if ref.ProjectID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("projID")
	}
	if ref.ID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("envID")
	}
	var dal store.Interface
	dal, err = store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return
	}
	summary, err := dal.LoadEnvironmentSummary(ref.ProjectID, ref.ID)
	return &summary, err
}
