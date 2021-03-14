package api

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
)

// GetEnvironmentSummary returns environment summary
func GetEnvironmentSummary(projID, envID string) (envSummary *models.EnvironmentSummary, err error) {
	if projID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("projID")
	}
	if envID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("envID")
	}
	summary, err := store.Current.LoadEnvironmentSummary(projID, envID)
	return &summary, err
}
