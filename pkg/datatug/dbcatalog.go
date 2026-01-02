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
	if err := v.ValidateWithOptions(false); err != nil {
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

var _ IProjectItems[*DbCatalog] = (DbCatalogs)(nil)

type DbCatalogs ProjectItems[*DbCatalog]

func (v DbCatalogs) IDs() []string {
	return ProjectItems[*DbCatalog](v).IDs()
}

func (v DbCatalogs) GetByID(id string) (t *DbCatalog) {
	return ProjectItems[*DbCatalog](v).GetByID(id)
}

func (v DbCatalogs) Validate() error {
	return ProjectItems[*DbCatalog](v).Validate()
}

// GetTable returns table
func (v DbCatalogs) GetTable(catalog, schema, name string) *CollectionInfo {
	for _, c := range v {
		if c.ID == catalog {
			for _, s := range c.Schemas {
				if s.ID == schema {
					for _, t := range s.Tables {
						if t.Name() == name {
							return t
						}
					}
				}
			}
		}
	}
	return nil
}

// DbCatalog hold info about DB database
type DbCatalog struct {
	DbCatalogBase
	Schemas DbSchemas
}

// ProjDbServerSummary holds summary info about DB server
type ProjDbServerSummary struct {
	ProjectItem
	DbServer ServerRef           `json:"dbServer"`
	Catalogs []*DbCatalogSummary `json:"databases,omitempty"`
}

// Validate returns error if not valid
func (v DbCatalog) Validate() error {
	if err := v.DbCatalogBase.Validate(); err != nil {
		return err
	}
	if err := v.Schemas.Validate(); err != nil {
		return err
	}
	return nil
}
