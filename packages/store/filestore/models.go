package filestore

import "github.com/datatug/datatug/packages/models"

// TableFile hold summary on table or view
type TableFile struct {
	models.TableProps
	PrimaryKey   *models.UniqueKey           `json:"primaryKey,omitempty"`
	ForeignKeys  []*models.ForeignKey        `json:"foreignKeys,omitempty"`
	ReferencedBy []*models.TableReferencedBy `json:"referencedBy,omitempty"`
	Columns      []*models.Column            `json:"columns,omitempty"`
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
	Columns []*models.Column `json:"columns,omitempty"`
}

// TablePropsFile info to be stored about table in a JSON file
type TablePropsFile struct {
	models.TableKey
	models.TableProps
}

// TableModelFile defines what to store in table model file
type TableModelFile struct {
	models.TableKey
}

// TableModelColumnsFile info to be stored about column in a JSON file
type TableModelColumnsFile struct {
	Columns models.ColumnModels `json:"columns,omitempty"`
}

// DbModelFile defines what to store to dbmodel file
type DbModelFile struct {
	models.ProjectItem
	Environments models.DbModelEnvironments `json:"environments,omitempty"`
}

// ProjDbServerFile stores info about project DB server
type ProjDbServerFile struct {
	models.ProjectItem
}

// DatabaseFile defines metadata to be stored in a JSON file in the db folder
type DatabaseFile struct {
	DbModel string `json:"dbmodel"`
}
