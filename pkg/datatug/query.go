package datatug

import (
	"fmt"
	"strings"
	"time"

	"github.com/strongo/validation"
)

// QueryFolders defines slice
type QueryFolders []*QueryFolder

// Validate returns error if not valid
func (v QueryFolders) Validate() error {
	for _, folder := range v {
		if err := folder.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// QueryFolder defines folder
type QueryFolder struct {
	ProjectItem
	Folders QueryFolders `json:"folders,omitempty" yaml:"folders,omitempty"`
	Items   QueryDefs    `json:"items,omitempty" yaml:"items,omitempty"`
}

// QueryFolderBrief defines brief for queries folder
type QueryFolderBrief struct {
	ProjItemBrief
	Folders []*QueryFolderBrief `json:"folders,omitempty" yaml:"folders,omitempty"`
	Items   []*QueryDefBrief    `json:"items,omitempty" yaml:"items,omitempty"`
}

// Validate returns error if not valid
func (v QueryFolderBrief) Validate() error {
	if err := v.ProjItemBrief.Validate(true); err != nil {
		return err
	}
	for i, folder := range v.Folders {
		if err := folder.Validate(); err != nil {
			return validation.NewErrBadRecordFieldValue(fmt.Sprintf("folders[%v]", i), err.Error())
		}
	}
	for i, item := range v.Items {
		if err := item.Validate(); err != nil {
			return validation.NewErrBadRecordFieldValue(fmt.Sprintf("items[%v]", i), err.Error())
		}
	}
	return nil
}

// Validate returns error if not valid
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

// QueryDefs defines slice
type QueryDefs []*QueryDef

// Validate returns error if not valid
func (v QueryDefs) Validate() error {
	for _, q := range v {
		if err := q.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type QueryDefBrief struct {
	ProjItemBrief
	Type  string `json:"type"` // Possible value: folder, SQL, GraphQL, etc.
	Draft bool   `json:"draft,omitempty" yaml:"draft,omitempty"`
}

func (v QueryDefBrief) Validate() error {
	if err := v.ProjItemBrief.Validate(true); err != nil {
		return err
	}
	if strings.TrimSpace(v.Type) == "" {
		return validation.NewErrRecordIsMissingRequiredField("type")
	}
	return nil
}

//func validateQueryBriefsMappedByID(queries map[string]*QueryDefBrief) error {
//	for id, query := range queries {
//		if err := validateItemMappedByID(id, query.GetID, query); err != nil {
//			return err
//		}
//	}
//	return nil
//}

// QueryDefWithFolderPath adds folder path to query definition
type QueryDefWithFolderPath struct {
	FolderPath string `json:"folderPath"`
	QueryDef
}

// Validate returns error if not valid
func (v QueryDefWithFolderPath) Validate() error {
	if v.FolderPath == "" {
		return validation.NewErrRecordIsMissingRequiredField("folderPath")
	}
	return v.QueryDef.Validate()
}

type QueryType string

const (
	QueryTypeSQL           QueryType = "SQL"
	QueryTypeHTTP          QueryType = "HTTP"
	QueryTypeStructuredSQL QueryType = "StructuredSQL"
)

func IsKnownQueryType(queryType QueryType) bool {
	switch queryType {
	case QueryTypeSQL, QueryTypeHTTP, QueryTypeStructuredSQL:
		return true
	default:
		return false
	}
}

// QueryDef holds query data
// For HTTP request host, port, etc, are stored in Targets property,
type QueryDef struct {
	ProjectItem
	Type       QueryType        `json:"type"` // Possible value: folder, SQL, GraphQL, HTTP, etc.
	Text       string           `json:"text,omitempty" yaml:"text,omitempty"`
	Draft      bool             `json:"draft,omitempty" yaml:"draft,omitempty"`
	Parameters Parameters       `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Targets    []QueryDefTarget `json:"targets,omitempty" yaml:"targets,omitempty"`
	// User might want to now what set of cols is returned even before hitting the RUN button.
	Recordsets []RecordsetDefinition `json:"recordsets,omitempty" yaml:"recordsets,omitempty"`
}

// QueryDefTarget defines target of query
type QueryDefTarget struct {
	Driver   string `json:"driver,omitempty" yaml:"driver,omitempty"`
	Catalog  string `json:"catalog,omitempty" yaml:"catalog,omitempty"`
	Protocol string `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	Host     string `json:"host,omitempty" yaml:"host,omitempty"`
	Port     int    `json:"port,omitempty" yaml:"port,omitempty"`
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
	case "HTTP":
		for i, target := range v.Targets {
			if target.Catalog != "" {
				return validation.NewErrBadRecordFieldValue(fmt.Sprintf("targets[%v]", i), "for HTTP queries catalog should be empty, got: %v"+target.Catalog)
			}
		}
	case "SQL", "GraphQL":
		//if strings.TrimSpace(v.Text) == "" {
		//	return validation.NewErrRequestIsMissingRequiredField("text")
		//}
	default:
		return validation.NewErrBadRecordFieldValue("type", "unsupported value: "+string(v.Type))
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
