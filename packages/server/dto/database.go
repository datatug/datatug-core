package dto

import "github.com/datatug/datatug/packages/models"

// DatabaseSummary holds database summary
type DatabaseSummary struct {
	models.ProjectEntity
	Environments []string       `json:"environments"`
	NumberOf     DatabaseCounts `json:"numberOf"`
}

// DatabaseCounts hold numbers about DB
type DatabaseCounts struct {
	Schemas int `json:"schemas"`
	Tables  int `json:"tables"`
	Views   int `json:"views"`
}

// DbCatalog hold info about DB database
type DbCatalog struct {
	Name string `json:"name"`
}

// ProjDbServerSummary holds summary info about DB server
type ProjDbServerSummary struct {
	models.ProjectEntity
	DbServer  models.DbServer   `json:"dbServer"`
	Databases []DatabaseSummary `json:"databases,omitempty"`
}
