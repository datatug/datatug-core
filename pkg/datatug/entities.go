package datatug

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/strongo/slice"
	"github.com/strongo/validation"
)

var _ IProjectItems[*Entity] = (Entities)(nil)

// Entities is a slice of *Entity
type Entities ProjectItems[*Entity]

func (v Entities) GetByID(id string) *Entity {
	return ProjectItems[*Entity](v).GetByID(id)
}

func (v Entities) IDs() []string {
	return ProjectItems[*Entity](v).IDs()
}

// Validate returns error if failed
func (v Entities) Validate() error {
	for i, item := range v {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("validation failed for entity at index=%v, id=%v: %w", i, item.ID, err)
		}
	}
	return nil
}

// ProjEntityBrief hold brief info about entity in project file
type ProjEntityBrief struct {
	ProjItemBrief
}

// Entity hold full info about entity
type Entity struct {
	ProjectItem
	//ProjEntityBrief
	ListOfTags
	Fields EntityFields `json:"fields,omitempty" firestore:"fields,omitempty"`
	Tables TableKeys    `json:"tables,omitempty" firestore:"tables,omitempty"`
}

// Validate returns error if not valid
func (v Entity) Validate() error {
	if err := v.ValidateWithOptions(false); err != nil {
		return err
	}
	if err := v.Fields.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("fields", err.Error())
	}
	if err := v.Tables.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("tables", err.Error())
	}
	if err := v.ListOfTags.Validate(); err != nil {
		return err
	}
	return nil
}

// StringPattern hold patterns for names
type StringPattern struct {
	Type          string `json:"type,omitempty"` // Options: regexp, exact
	Value         string `json:"value,omitempty"`
	CaseSensitive bool   `json:"caseSensitive,omitempty"`
}

// StringPatterns is a slice of *StringPattern
type StringPatterns []*StringPattern

// Validate returns error if not valid
func (v StringPatterns) Validate() error {
	for i, p := range v {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("invalid pattern at index %v: %w", i, err)
		}
	}
	return nil
}

// Validate returns error if not valid
func (v StringPattern) Validate() error {
	switch v.Type {
	case "":
		return validation.NewErrRequestIsMissingRequiredField("type")
	case "exact":
		// OK
	case "regexp":
		if _, err := regexp.Compile(v.Value); err != nil {
			return validation.NewErrBadRecordFieldValue("regexp", err.Error())
		}
	default:
		return validation.NewErrBadRecordFieldValue("type", fmt.Sprintf("unknown value: %v", v.Type))
	}
	if v.Value == "" {
		return validation.NewErrRequestIsMissingRequiredField("value")
	}
	return nil
}

// EntityField hold info about entity field
type EntityField struct {
	ID           string         `json:"id" firestore:"id"`
	Type         string         `json:"type" firestore:"type"`
	Title        string         `json:"title,omitempty" firestore:"title,omitempty"`
	IsKeyField   bool           `json:"isKeyField,omitempty" firestore:"isKeyField,omitempty"`
	NamePatterns StringPatterns `json:"namePatterns" firestore:"namePattern"`
	// Mappings holds declared physical columns this field maps to. Declared
	// mappings are authoritative; when empty, a resolver may derive one from
	// NamePatterns and must label it "inferred".
	Mappings PhysicalRefs `json:"mappings,omitempty" firestore:"mappings,omitempty"`
}

// Validate returns error if not valid
func (v EntityField) Validate() error {
	if v.ID == "" {
		return validation.NewErrRequestIsMissingRequiredField("id")
	}
	if v.Type == "" {
		return validation.NewErrRequestIsMissingRequiredField("type")
	}
	if slice.Index(KnownTypes, v.Type) < 0 {
		return validation.NewErrBadRecordFieldValue("type", fmt.Sprintf("unknown field type: %v: expected one of: %v", v.Type, strings.Join(KnownTypes, ", ")))
	}
	if err := v.NamePatterns.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("namePatterns", err.Error())
	}
	if err := v.Mappings.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("mappings", err.Error())
	}
	return nil
}

// PhysicalRef references a physical column an entity field maps to.
type PhysicalRef struct {
	Source     string `json:"source" firestore:"source"`
	Collection string `json:"collection" firestore:"collection"`
	Column     string `json:"column" firestore:"column"`
}

// Validate returns error if not valid
func (v PhysicalRef) Validate() error {
	if strings.TrimSpace(v.Source) == "" {
		return validation.NewErrRecordIsMissingRequiredField("source")
	}
	if strings.TrimSpace(v.Collection) == "" {
		return validation.NewErrRecordIsMissingRequiredField("collection")
	}
	if strings.TrimSpace(v.Column) == "" {
		return validation.NewErrRecordIsMissingRequiredField("column")
	}
	return nil
}

// PhysicalRefs is a slice of PhysicalRef
type PhysicalRefs []PhysicalRef

// Validate returns error if not valid: every ref must be valid and, since a
// field can only mean one thing per table, unique per source+collection.
func (v PhysicalRefs) Validate() error {
	seen := make(map[string]struct{}, len(v))
	for i, ref := range v {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("mappings[%v]: %w", i, err)
		}
		key := ref.Source + "\x00" + ref.Collection
		if _, ok := seen[key]; ok {
			return fmt.Errorf("mappings[%v]: duplicate mapping for source=%v, collection=%v", i, ref.Source, ref.Collection)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// EntityFields is a slice of EntityField
type EntityFields []*EntityField

// Validate returns error if not valid
func (v EntityFields) Validate() error {
	for i, f := range v {
		if err := f.Validate(); err != nil {
			return fmt.Errorf("fields[%v]: %w", i, err)
		}
	}
	return nil
}

// EntityFieldRef holds reference to entity field
type EntityFieldRef struct {
	Entity string `json:"entity"`
	Field  string `json:"field"`
}

// Validate returns error if not valid
func (v EntityFieldRef) Validate() error {
	if strings.TrimSpace(v.Entity) == "" {
		return validation.NewErrRecordIsMissingRequiredField("entity")
	}
	if strings.TrimSpace(v.Field) == "" {
		return validation.NewErrRecordIsMissingRequiredField("field")
	}
	return nil
}
