package models

import (
	"fmt"
	"github.com/strongo/validation"
	"strings"
)

// Database hold information about a database
type DbCatalog struct {
	ProjectItem
	DbModel string `json:"dbModel"`
	Schemas DbSchemas
}

// Validate returns error if not valid
func (v DbCatalog) Validate() error {
	if err := v.ProjectItem.Validate(false); err != nil {
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
	ProjectItem
	Tables []*Table `json:"tables"`
	Views  []*Table `json:"views"`
}

// Validate returns error if not valid
func (v DbSchema) Validate() error {
	if err := v.ProjectItem.Validate(false); err != nil {
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

// DbCatalogs is a slice of pointers to Database
type DbCatalogs []*DbCatalog

func (v DbCatalogs) GetTable(catalog, schema, name string) *Table {
	for _, c := range v {
		if c.ID == catalog {
			for _, s := range c.Schemas {
				if s.ID == schema {
					for _, t := range s.Tables {
						if t.Name == name {
							return t
						}
					}
				}
			}
		}
	}
	return nil
}

// Validate returns error if failed
func (v DbCatalogs) Validate() error {
	for i, db := range v {
		if err := db.Validate(); err != nil {
			return fmt.Errorf("validaiton failed for db at index %v: %w", i, err)
		}
	}
	return nil
}

// GetDbByID returns Database by ID
func (v DbCatalogs) GetDbByID(id string) *DbCatalog {
	for _, db := range v {
		if strings.EqualFold(db.ID, id) {
			return db
		}
	}
	return nil
}

// TableKeys is a []TableKey
type TableKeys []TableKey

// Validate returns error if not valid
func (v TableKeys) Validate() error {
	for i, t := range v {
		if err := t.Validate(); err != nil {
			return fmt.Errorf("invalid table key at %v: %w", i, err)
		}
	}
	return nil
}

// TableKey defines a key that identifies a table or a view
type TableKey struct {
	Name    string `json:"name"`
	Schema  string `json:"schema,omitempty"`
	Catalog string `json:"catalog,omitempty"`
}

func (v TableKey) String() string {
	if v.Schema == "" && v.Catalog == "" {
		return v.Name
	}
	if v.Catalog == "" {
		return fmt.Sprintf("%v.%v", v.Schema, v.Name)
	}
	return fmt.Sprintf("%v.%v.%v", v.Catalog, v.Schema, v.Name)
}

// Validate return error if not valid
func (v TableKey) Validate() error {
	if v.Name == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	return nil
}

// TableProps holds properties of a table
type TableProps struct {
	DbType     string       `json:"dbType,omitempty"` // e.g. "BASE TABLE", "VIEW", etc.
	UniqueKeys []*UniqueKey `json:"uniqueKeys,omitempty"`
}

// Validate return error if not valid
func (v TableProps) Validate() error {
	switch v.DbType {
	case "":
		return validation.NewErrRecordIsMissingRequiredField("dbType")
	case "BASE TABLE", "VIEW":
	default:
		return fmt.Errorf("unknown dbType: %v", v.DbType)
	}
	for i, uk := range v.UniqueKeys {
		if err := uk.Validate(); err != nil {
			return fmt.Errorf("invalid unique key at index %v: %w", i, err)
		}
	}
	return nil
}

// UniqueKey holds metadata about unique key
type UniqueKey struct {
	Name        string   `json:"name"`
	Columns     []string `json:"columns"`
	IsClustered bool     `json:"isClustered,omitempty"`
}

// Index holds info about DB table index
type Index struct {
	Name               string         `json:"name"`
	Type               string         `json:"type"`
	Columns            []*IndexColumn `json:"columns"`
	IsClustered        bool           `json:"clustered,omitempty"`
	IsXml              bool           `json:"xml,omitempty"`
	IsColumnStore      bool           `json:"columnstore,omitempty"`
	IsHash             bool           `json:"hash,omitempty"`
	IsUnique           bool           `json:"unique,omitempty"`
	IsUniqueConstraint bool           `json:"uniqueConstraint,omitempty"`
	IsPrimaryKey       bool           `json:"primaryKey,omitempty"`
}

// IndexColumn holds info about a col in a DB table index
type IndexColumn struct {
	Name             string `json:"name"`
	IsDescending     bool   `json:"descending,omitempty"`
	IsIncludedColumn bool   `json:"included,omitempty"`
}

// Validate returns error if not valid
func (v *UniqueKey) Validate() error {
	if v == nil {
		return nil
	}
	if v.Name == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	if len(v.Columns) == 0 {
		return validation.NewErrRecordIsMissingRequiredField("columns")
	}
	for i, col := range v.Columns {
		if col == "" {
			return validation.NewErrBadRecordFieldValue("columns", fmt.Sprintf("empty column name at index %v", i))
		}
	}
	return nil
}

// ForeignKey holds metadata about foreign key
type ForeignKey struct {
	Name        string   `json:"name"`
	Columns     []string `json:"columns"`
	RefTable    TableKey `json:"refTable"`
	MatchOption string   `json:"matchOption,omitempty"` // Document what this?
	UpdateRule  string   `json:"updateRule,omitempty"`  // Document what this?
	DeleteRule  string   `json:"deleteRule,omitempty"`  // Document what this?
}

// Validate returns error if not valid
func (v ForeignKey) Validate() error {
	if v.Name == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	if len(v.Columns) == 0 {
		return validation.NewErrRecordIsMissingRequiredField("columns")
	}
	return nil
}

// RefByForeignKey holds metadata about reference by FK
type RefByForeignKey struct {
	Name        string   `json:"name"`
	Columns     []string `json:"columns"`
	MatchOption string   `json:"matchOption,omitempty"`
	UpdateRule  string   `json:"updateRule,omitempty"`
	DeleteRule  string   `json:"deleteRule,omitempty"`
}

// Tables is a slice of *Table
type Tables []*Table

type Constraint struct {
	Name string
	Type string
}

// GetByKey return a *Table by key or nil if not found
func (v Tables) GetByKey(k TableKey) *Table {
	for _, t := range v {
		if t.TableKey == k {
			return t
		}
	}
	return nil
}

// RecordsetBaseDef is used by: Table, RecordsetDefinition
type RecordsetBaseDef struct {
	PrimaryKey    *UniqueKey    `json:"primaryKey,omitempty"`
	ForeignKeys   []*ForeignKey `json:"foreignKeys,omitempty"`
	AlternateKeys []UniqueKey   `json:"alternateKey,omitempty"`
	ActiveIssues  *Issues       `json:"issues,omitempty"`
}

// Table holds metadata about a table or view
type Table struct {
	RecordsetBaseDef
	TableKey
	TableProps
	Columns      []*TableColumn       `json:"columns,omitempty"`
	Indexes      []*Index             `json:"indexes,omitempty"`
	ReferencedBy []*TableReferencedBy `json:"referencedBy,omitempty"`
	RecordsCount *int                 `json:"recordsCount,omitempty"`
}

// Validate returns error if not valid
func (v Table) Validate() error {
	if err := v.TableKey.Validate(); err != nil {
		return err
	}
	if err := v.TableProps.Validate(); err != nil {
		return err
	}
	if err := v.PrimaryKey.Validate(); err != nil {
		return fmt.Errorf("invalid primary key: %w", err)
	}
	for i, col := range v.Columns {
		if err := col.Validate(); err != nil {
			return fmt.Errorf("invalid column at index %v: %w", i, err)
		}
	}
	for i, fk := range v.ForeignKeys {
		if fk == nil {
			return validation.NewErrBadRecordFieldValue("foreignKeys", fmt.Sprintf("nil value at index %v", i))
		}
		if err := fk.Validate(); err != nil {
			return fmt.Errorf("invalid foreign key at index %v: %w", i, err)
		}
	}
	return nil
}

// TableReferencedBy holds metadata about table/view that reference a table/view
type TableReferencedBy struct {
	TableKey
	ForeignKeys []*RefByForeignKey `json:"foreignKeys"`
}

// DbColumnProps holds column metadata
type DbColumnProps struct {
	Name              string        `json:"name"`
	OrdinalPosition   int           `json:"ordinalPosition"`
	IsNullable        bool          `json:"isNullable"`
	DbType            string        `json:"dbType"`
	Default           *string       `json:"default,omitempty"`
	CharMaxLength     *int          `json:"charMaxLength,omitempty"`
	CharOctetLength   *int          `json:"charOctetLength,omitempty"`
	DateTimePrecision *int          `json:"dateTimePrecision,omitempty"`
	CharacterSet      *CharacterSet `json:"characterSet,omitempty"`
	Collation         *Collation    `json:"collation,omitempty"`
}

// Validate return error if not valid
func (v DbColumnProps) Validate() error {
	if v.Name == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	if v.OrdinalPosition < 0 {
		return validation.NewErrBadRecordFieldValue("ordinalPosition", fmt.Sprintf("should be positive, got: %v", v.OrdinalPosition))
	}
	if v.DateTimePrecision != nil && *v.DateTimePrecision < 0 {
		return validation.NewErrBadRecordFieldValue("dateTimePrecision", fmt.Sprintf("should be positive, got: %v", *v.DateTimePrecision))
	}
	if v.CharMaxLength != nil && *v.CharMaxLength < -1 {
		return validation.NewErrBadRecordFieldValue("charMaxLength", fmt.Sprintf("should be positive, got: %v", *v.CharMaxLength))
	}
	if v.CharOctetLength != nil && *v.CharOctetLength < -1 {
		return validation.NewErrBadRecordFieldValue("charOctetLength", fmt.Sprintf("should be positive, got: %v", *v.CharOctetLength))
	}
	if v.CharacterSet != nil {
		if err := v.CharacterSet.Validate(); err != nil {
			return fmt.Errorf("invalid characterSet: %w", err)
		}
	}
	if v.Collation != nil {
		if err := v.Collation.Validate(); err != nil {
			return fmt.Errorf("invalid collation: %w", err)
		}
	}
	return nil
}

// TableColumn holds col metadata
type TableColumn struct {
	DbColumnProps
	//ChangeType ChangeType `json:"-"` // Document what it is and why needed
	//ByEnv       map[string]TableColumn `json:"byEnv,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
}

// ColumnModel defines column as we expect it to be
type ColumnModel struct {
	TableColumn
	ByEnv  StateByEnv `json:"byEnv,omitempty"`
	Checks Checks     `json:"checks,omitempty"`
}

// Validate returns error if not valid
func (v *ColumnModel) Validate() error {
	if err := v.TableColumn.Validate(); err != nil {
		return err
	}
	if err := v.ByEnv.Validate(); err != nil {
		return err
	}
	if err := v.Checks.Validate(); err != nil {
		return err
	}
	return nil
}

// ColumnModels is a slice of *ColumnModel
type ColumnModels []*ColumnModel

// Validate returns error if not valid
func (v ColumnModels) Validate() error {
	for _, c := range v {
		if err := c.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Collation holds info about collation
type Collation struct {
	Catalog string `json:"catalog,omitempty"`
	Schema  string `json:"schema,omitempty"`
	Name    string `json:"name"`
}

// Validate return error if not valid
func (v Collation) Validate() error {
	if v.Name == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	return nil
}

// CharacterSet holds info about character set
type CharacterSet struct {
	Catalog string `json:"Catalog,omitempty"`
	Schema  string `json:"Schema,omitempty"`
	Name    string `json:"ID"`
}

// Validate returns error if not valid
func (v CharacterSet) Validate() error {
	if v.Name == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	return nil
}

// Validate return error if not valid
func (v TableColumn) Validate() error {
	if err := v.DbColumnProps.Validate(); err != nil {
		if v.Name == "" {
			return err
		}
		return fmt.Errorf("invalid column [%v]: %w", v.Name, err)
	}
	return nil
}
