package dto

import (
	"github.com/datatug/datatug/packages/models"
)

// EnvironmentSummary holds environment summary
type EnvironmentSummary struct {
	models.ProjectEntity
	Servers   []models.EnvDbServer `json:"servers"`
	Databases []EnvDb              `json:"databases"`
}

// EnvDb hold info about DB in specific environment
type EnvDb struct {
	models.ProjectEntity
	Server models.DbServer `json:"server"`
}
