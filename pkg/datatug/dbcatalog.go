package datatug

import "github.com/strongo/validation"

// DbCatalogBase defines base data for DB catalog
type DbCatalogBase struct {
	ProjectItem
	Driver  string `json:"driver"`
	Path    string `json:"path,omitempty"` // for SQLite
	DbModel string `json:"dbModel"`
}

// Validate returns error if not valid
func (v DbCatalogBase) Validate() error {
	if err := v.ProjectItem.Validate(false); err != nil {
		return err
	}
	if v.Driver == "" {
		return validation.NewErrRecordIsMissingRequiredField("driver")
	}
	if v.Driver == "sqlite3" && v.Path == "" {
		return validation.NewErrRecordIsMissingRequiredField("path")
	}
	return nil
}

// DbCatalogSummary holds database summary
type DbCatalogSummary struct {
	DbCatalogBase
	Environments []string         `json:"environments"`
	NumberOf     *DbCatalogCounts `json:"numberOf"`
}

// DbCatalogCounts hold numbers about DB
type DbCatalogCounts struct {
	Schemas int `json:"schemas"`
	Tables  int `json:"tables"`
	Views   int `json:"views"`
}

// EnvDbCatalog hold info about DB database
type EnvDbCatalog struct {
	DbCatalogBase
	Schemas DbSchemas
}

// ProjDbServerSummary holds summary info about DB server
type ProjDbServerSummary struct {
	ProjectItem
	DbServer ServerReference     `json:"dbServer"`
	Catalogs []*DbCatalogSummary `json:"databases,omitempty"`
}

// Validate returns error if not valid
func (v EnvDbCatalog) Validate() error {
	if err := v.DbCatalogBase.Validate(); err != nil {
		return err
	}
	if err := v.Schemas.Validate(); err != nil {
		return err
	}
	return nil
}
