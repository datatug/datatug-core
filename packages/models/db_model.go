package models

import (
	"fmt"
)

// DbModels is a slice of *DbModel
type DbModels []*DbModel

// GetDbModelByID return DB model by ID
func (v DbModels) GetDbModelByID(id string) (dbModel *DbModel) {
	for _, dbModel = range v {
		if dbModel.ID == id {
			return
		}
	}
	return nil
}

// Validate returns error if failed
func (v DbModels) Validate() error {
	for i, item := range v {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("validation failed for db model at index=%v, id=%v: %w", i, item.ID, err)
		}
	}
	return nil
}

// IDs returns slice of IDs of db models
func (v DbModels) IDs() (ids []string) {
	if len(v) == 0 {
		return
	}
	ids = make([]string, len(v))
	for i, dbModel := range v {
		ids[i] = dbModel.ID
	}
	return
}

// DbModel holds a model of a database
type DbModel struct {
	ProjectEntity
	Schemas      SchemaModels        `json:"schemas,omitempty"`
	Environments DbModelEnvironments `json:"environments,omitempty"`
}

// DbModelEnvironments slice of *DbModelEnv
type DbModelEnvironments []*DbModelEnv

// GetByID return *DbModelEnv by ID
func (v DbModelEnvironments) GetByID(id string) *DbModelEnv {
	for _, v := range v {
		if v.ID == id {
			return v
		}
	}
	return nil
}

// DbModelEnv holds links from db model to environments
type DbModelEnv struct {
	ID        string `json:"id"` // environment ID
	Databases DbModelDatabases
}

// DbModelDatabases slice of *DbModelDb
type DbModelDatabases []*DbModelDb

// GetByID returns DbModelDb by ID
func (v DbModelDatabases) GetByID(id string) (dbModelDb *DbModelDb) {
	for _, dbModelDb = range v {
		if dbModelDb.ID == id {
			return dbModelDb
		}
	}
	return nil
}

// DbModelDb holds DB model
type DbModelDb struct {
	ID string `json:"id"`
}

// Validate returns error if not valid
func (v DbModel) Validate() error {
	if err := v.ProjectEntity.Validate(false); err != nil {
		return err
	}
	for i, schema := range v.Schemas {
		if err := schema.Validate(); err != nil {
			return fmt.Errorf("invalid schema at index=%v, id=%v: %w", i, schema.ID, err)
		}
	}
	return nil
}

// SchemaModels is a slice of *SchemaModel
type SchemaModels []*SchemaModel

// GetByID return *SchemaModel by ID
func (v SchemaModels) GetByID(id string) (schemaModel *SchemaModel) {
	for _, schemaModel = range v {
		if schemaModel.ID == id {
			return
		}
	}
	return nil
}

// SchemaModel holds model for a DB schema
type SchemaModel struct {
	ProjectEntity
	Tables TableModels `json:"tables"`
	Views  TableModels `json:"views"`
}

// Validate returns error if not valid
func (v SchemaModel) Validate() error {
	if err := v.ProjectEntity.Validate(false); err != nil {
		return err
	}
	for i, table := range v.Tables {
		if err := table.Validate(); err != nil {
			return fmt.Errorf("invalid table model at index=%v, name=%v: %w", i, table.Name, err)
		}
	}
	for i, view := range v.Views {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("invalid view model at index=%v, name=%v: %w", i, view.Name, err)
		}
	}
	return nil
}

// TableModels is a slice of *TableModel
type TableModels []*TableModel

// GetByKey returns table by key (name, schema, catalog)
func (v TableModels) GetByKey(k TableKey) *TableModel {
	for _, t := range v {
		if t.TableKey == k {
			return t
		}
	}
	return nil
}

// GetByName returns table by name
func (v TableModels) GetByName(name string) *TableModel {
	for _, t := range v {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// TableModel hold models for table or view
type TableModel struct {
	TableKey
	DbType  string `json:"dbType,omitempty"` // e.g. "BASE TABLE", "VIEW", etc.
	Columns ColumnModels
	Checks  Checks     `json:"checks,omitempty"` // References to checks by type/id
	ByEnv   StateByEnv `json:"byEnv,omitempty"`
}

// Validate returns error if not valid
func (v *TableModel) Validate() error {
	if err := v.TableKey.Validate(); err != nil {
		return err
	}
	if v.Columns != nil {
		if err := v.Columns.Validate(); err != nil {
			return err
		}
	}
	if v.ByEnv != nil {
		if err := v.ByEnv.Validate(); err != nil {
			return err
		}
	}
	if v.Checks != nil {
		if err := v.Checks.Validate(); err != nil {
			return fmt.Errorf("table %v has invalid checks: %w", v.TableKey.String(), err)
		}
	}
	return nil
}

// ValueRegexCheck holds regex to check
type ValueRegexCheck struct {
	Regex string `json:"regex"`
}
