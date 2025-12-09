package filestore

import (
	"fmt"

	"github.com/datatug/datatug-core/pkg/models"
	"github.com/strongo/validation"
)

// TableFile hold summary on table or view
type TableFile struct {
	models.TableProps
	PrimaryKey   *models.UniqueKey           `json:"primaryKey,omitempty"`
	ForeignKeys  []*models.ForeignKey        `json:"foreignKeys,omitempty"`
	ReferencedBy []*models.TableReferencedBy `json:"referencedBy,omitempty"`
	Columns      []*models.TableColumn       `json:"columns,omitempty"`
	Indexes      []*models.Index             `json:"indexes,omitempty"`
}

// TableRefsByFile info to be stored about reference in a JSON file
type TableRefsByFile struct {
	models.TableKey
	ReferencedBy []*models.TableReferencedBy `json:"referencedBy"`
}

// TableForeignKeysFile info to be stored about FK in a JSON file
type TableForeignKeysFile struct {
	models.TableKey
	ForeignKeys []*models.ForeignKey `json:"foreignKeys"`
}

// TablePrimaryKeyFile info to be stored about PK in a JSON file
type TablePrimaryKeyFile struct {
	models.TableKey
	PrimaryKey *models.UniqueKey `json:"primaryKey"`
}

// TableColumnsFile info to be stored about column in a JSON file
type TableColumnsFile struct {
	models.TableKey
	Columns []*models.TableColumn `json:"columns,omitempty"`
}

// TablePropsFile info to be stored about table in a JSON file
type TablePropsFile struct {
	models.TableKey
	models.TableProps
}

// TableModelFile defines what to storage in table model file
type TableModelFile struct {
	models.TableKey
}

// TableModelColumnsFile info to be stored about column in a JSON file
type TableModelColumnsFile struct {
	Columns models.ColumnModels `json:"columns,omitempty"`
}

// Validate returns error if not valid
func (v TableModelColumnsFile) Validate() error {
	for i, c := range v.Columns {
		if err := c.Validate(); err != nil {
			return fmt.Errorf("invalid column at index %v: %w", i, err)
		}
	}
	return nil
}

// DbModelFile defines what to storage to dbmodel file
type DbModelFile struct {
	models.ProjectItem
	Environments models.DbModelEnvironments `json:"environments,omitempty"`
}

// Validate returns error if not valid
func (v DbModelFile) Validate() error {
	if err := v.ProjectItem.Validate(false); err != nil {
		return err
	}
	if err := v.Environments.Validate(); err != nil {
		return err
	}
	return nil
}

// ProjDbServerFile stores info about project DB server
type ProjDbServerFile struct {
	models.ProjectItem
}

// DbCatalogFile defines metadata to be stored in a JSON file in the db folder
type DbCatalogFile struct {
	Driver  string `json:"driver"` // It's excessive but good to have for validation
	Path    string `json:"path,omitempty"`
	DbModel string `json:"dbmodel,omitempty"`
}

// Validate returns error if not valid
func (v DbCatalogFile) Validate() error {
	if v.Driver == "" {
		return validation.NewErrRecordIsMissingRequiredField("driver")
	}
	if v.Driver == "sqlite3" && v.Path == "" {
		return validation.NewErrRecordIsMissingRequiredField("path")
	}
	return nil
}
