package models

import (
	"fmt"
	"github.com/strongo/validation"
	"time"
)

type QueryFolders []QueryFolder

func (v QueryFolders) Validate() error {
	for _, folder := range v {
		if err := folder.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type QueryFolder struct {
	ProjectItem
	Folders QueryFolders `json:"folders,omitempty" yaml:"folders,omitempty"`
	Items   QueryDefs    `json:"items,omitempty" yaml:"items,omitempty"`
}

func (v QueryFolder) Validate() error {
	if err := v.ProjectItem.Validate(false); err != nil {
		return err
	}
	if err := v.Folders.Validate(); err != nil {
		return err
	}
	if err := v.Items.Validate(); err != nil {
		return err
	}
	return nil
}

type QueryDefs []QueryDef

func (v QueryDefs) Validate() error {
	for _, q := range v {
		if err := q.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// QueryDef holds query data
type QueryDef struct {
	ProjectItem
	Type       string           `json:"type"` // Possible value: folder, SQL, GraphQL, etc.
	Text       string           `json:"text,omitempty" yaml:"text,omitempty"`
	Draft      bool             `json:"draft,omitempty" yaml:"draft,omitempty"`
	Parameters Parameters       `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Targets    []QueryDefTarget `json:"targets,omitempty" yaml:"targets,omitempty"`
	// User might want to now what set of cols is returned even before hitting the RUN button.
	Recordsets []RecordsetDefinition `json:"recordsets,omitempty" yaml:"recordsets,omitempty"`
}

type QueryDefTarget struct {
	Driver  string `json:"driver,omitempty" yaml:"driver,omitempty"`
	Catalog string `json:"catalog,omitempty" yaml:"catalog,omitempty"`
	Host    string `json:"host,omitempty" yaml:"host,omitempty"`
	Port    int    `json:"port,omitempty" yaml:"port,omitempty"`
	Credentials
}

// Validate returns error if not valid
func (v QueryDef) Validate() error {
	if err := v.ProjectItem.Validate(true); err != nil {
		return err
	}
	switch v.Type {
	case "":
		return validation.NewErrRequestIsMissingRequiredField("type")
	case "folder":
		if v.Text != "" {
			return validation.NewErrBadRecordFieldValue("text", "should be empty for folders")
		}
	case "sql", "graphql":
		//if strings.TrimSpace(v.Text) == "" {
		//	return validation.NewErrRequestIsMissingRequiredField("text")
		//}
	default:
		return validation.NewErrBadRecordFieldValue("type", "unsupported value: "+v.Type)
	}
	if err := v.Parameters.Validate(); err != nil {
		return err
	}
	return nil
}

// QueryResult holds results of a query execution
type QueryResult struct {
	Created       time.Time   `json:"created"`
	EnvironmentID string      `json:"env"`
	Driver        string      `json:"driver"`
	Target        string      `json:"target"`
	Recordsets    []Recordset `json:"recordset,omitempty"`
}

// Validate returns error if not valid
func (v QueryResult) Validate() error {
	if v.Created.IsZero() {
		return validation.NewErrRecordIsMissingRequiredField("created")
	}
	if v.Target == "" {
		return validation.NewErrRecordIsMissingRequiredField("target")
	}
	if v.Driver == "" {
		return validation.NewErrRecordIsMissingRequiredField("driver")
	}
	for i, recordset := range v.Recordsets {
		if err := recordset.Validate(); err != nil {
			return validation.NewErrBadRecordFieldValue(fmt.Sprintf("recordsets[%v]", i), err.Error())
		}
	}
	return nil
}
