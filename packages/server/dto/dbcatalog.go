package dto

import "github.com/datatug/datatug/packages/models"

type DbCatalogBase struct {
	models.ProjectItem
	Path string `json:"path"`
}

// DbCatalogSummary holds database summary
type DbCatalogSummary struct {
	DbCatalogBase
	Environments []string        `json:"environments"`
	NumberOf     DbCatalogCounts `json:"numberOf"`
}

// DbCatalogCounts hold numbers about DB
type DbCatalogCounts struct {
	Schemas int `json:"schemas"`
	Tables  int `json:"tables"`
	Views   int `json:"views"`
}

// DbCatalog hold info about DB database
type DbCatalog struct {
	DbCatalogBase
}

// ProjDbServerSummary holds summary info about DB server
type ProjDbServerSummary struct {
	models.ProjectItem
	DbServer models.ServerReference `json:"dbServer"`
	Catalogs []*DbCatalogSummary     `json:"databases,omitempty"`
}
