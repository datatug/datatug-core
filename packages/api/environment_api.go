package api

import (
	"github.com/datatug/datatug/packages/server/dto"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
)

// GetEnvironmentSummary returns environment summary
func GetEnvironmentSummary(projID, envID string) (envSummary *dto.EnvironmentSummary, err error) {
	if projID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("projID")
	}
	if envID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("envID")
	}
	summary, err := store.Current.GetEnvironmentSummary(projID, envID)
	return &summary, err
}
