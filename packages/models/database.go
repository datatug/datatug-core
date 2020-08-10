package models

import (
	"fmt"
	"strings"
)

// Database hold information about a database
type Database struct {
	ProjectEntity
	DbModel string `json:"dbModel"`
	Schemas DbSchemas
}

// Validate returns error if not valid
func (v Database) Validate() error {
	if err := v.ProjectEntity.Validate(false); err != nil {
		return err
	}
	if err := v.Schemas.Validate(); err != nil {
		return err
	}
	return nil
}

// DbSchemas is a slice of *DbSchema
type DbSchemas []*DbSchema

// Validate returns error if not valid
func (v DbSchemas) Validate() error {
	for i, schema := range v {
		if err := schema.Validate(); err != nil {
			return fmt.Errorf("invalid schema at index=%v, id=%v: %w", i, schema.ID, err)
		}
	}
	return nil
}

// GetByID returns schema by ID
func (v DbSchemas) GetByID(id string) *DbSchema {
	for _, schema := range v {
		if schema.ID == id {
			return schema
		}
	}
	return nil
}

// DbSchema represents a schema in a database
type DbSchema struct {
	ProjectEntity
	Tables []*Table `json:"tables"`
	Views  []*Table `json:"views"`
}

// Validate returns error if not valid
func (v DbSchema) Validate() error {
	if err := v.ProjectEntity.Validate(false); err != nil {
		return err
	}
	for i, t := range v.Tables {
		if err := t.Validate(); err != nil {
			return fmt.Errorf("invalid table at index %v: %w", i, err)
		}
	}
	for i, t := range v.Views {
		if err := t.Validate(); err != nil {
			return fmt.Errorf("invalid view at index %v: %w", i, err)
		}
	}
	return nil
}

// Databases is a slice of pointers to Database
type Databases []*Database

// Validate returns error if failed
func (v Databases) Validate() error {
	for i, db := range v {
		if err := db.Validate(); err != nil {
			return fmt.Errorf("validaiton failed for db at index %v: %w", i, err)
		}
	}
	return nil
}

// GetDbByID returns Database by ID
func (v Databases) GetDbByID(id string) *Database {
	for _, db := range v {
		if strings.EqualFold(db.ID, id) {
			return db
		}
	}
	return nil
}
