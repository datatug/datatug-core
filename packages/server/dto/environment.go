package dto

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/validation"
)

// EnvironmentSummary holds environment summary
type EnvironmentSummary struct {
	models.ProjectItem
	Servers   []models.EnvDbServer `json:"servers"`
	Databases []EnvDb              `json:"databases"`
}

// EnvDb hold info about DB in specific environment
type EnvDb struct {
	models.ProjectItem
	DbModel string                 `json:"dbModel"`
	Server  models.ServerReference `json:"server"`
}

func (v EnvDb) Validate() error {
	if err := v.ProjectItem.Validate(false); err != nil {
		return err
	}
	if err := v.Server.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("server", err.Error())
	}
	return nil
}
