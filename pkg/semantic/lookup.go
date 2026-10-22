package semantic

import (
	"sort"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// KeyKind is the kind of value a lookup source's key column holds. Its
// values match the contract's TypedValue types, so a picked key can be
// returned as {type: KeyKind, value: ...}.
type KeyKind string

const (
	// KeyInteger is a key column with integer affinity.
	KeyInteger KeyKind = "integer"
	// KeyText is a key column with text affinity.
	KeyText KeyKind = "string"
)

// maxDeclaredDisplay and maxInferredDisplay bound the display columns of one
// lookup source (backstage parameter-lookup REQ:display-columns).
const (
	maxDeclaredDisplay = 6
	maxInferredDisplay = 3
)

// DisplayColumn is one column shown beside the key in a lookup picker.
type DisplayColumn struct {
	// Column is the physical column name in the lookup source's collection.
	Column string
	// Field is the entity field the column comes from; empty for a column
	// inferred from the schema alone.
	Field string
	// Label is the field's title, its ID, or (for a schema-only column) the
	// column name.
	Label string
	// Provenance is Declared for a column listed in Entity.Display and
	// Inferred for the fallback columns.
	Provenance Provenance
}

// LookupSource is one collection a parameter's value can be picked from: its
// single-column primary key is declared to carry the parameter's meaning.
type LookupSource struct {
	Source     string
	Collection string
	KeyColumn  string
	KeyKind    KeyKind
	Display    []DisplayColumn
	// DisplayProvenance is Declared when Display came from Entity.Display and
	// Inferred when it was guessed (the picker must say so). It is Declared
	// for an empty Display, since nothing was guessed.
	DisplayProvenance Provenance
}

// ColumnKeyKind classifies a declared column type by SQLite-style affinity:
// a type containing "INT" is an integer key, one containing "CHAR", "CLOB"
// or "TEXT" (or the DataTug type "string") is a text key. Any other type -
// decimal, real, date, blob - cannot be a lookup key and yields "".
func ColumnKeyKind(columnType string) KeyKind {
	t := strings.ToUpper(strings.TrimSpace(columnType))
	switch {
	case strings.Contains(t, "INT"):
		return KeyInteger
	case strings.Contains(t, "CHAR"), strings.Contains(t, "CLOB"), strings.Contains(t, "TEXT"), t == "STRING":
		return KeyText
	default:
		return ""
	}
}

// ParameterAcceptsKey reports whether a parameter of parameterType can hold a
// key of kind: a string parameter takes both kinds (an integer key as decimal
// text), an integer parameter only integer keys, and no other type takes any.
func ParameterAcceptsKey(parameterType string, kind KeyKind) bool {
	switch parameterType {
	case "string":
		return kind == KeyInteger || kind == KeyText
	case "integer":
		return kind == KeyInteger
	default:
		return false
	}
}

// LookupSources returns the collections the value of parameter can be picked
// from (backstage parameter-lookup REQ:lookup-source-resolution): every
// declared mapping (EntityField.Mappings, never a NamePatterns inference) of
// the field parameter.Meta names, whose collection's schema in
// schemaByCollection has a primary key of exactly one column equal to the
// mapped column, with a key kind (ColumnKeyKind of that column's type in
// TableSchema.Columns, or of the entity field's type when the schema lists no
// columns) that parameter's type can hold (ParameterAcceptsKey).
//
// A parameter with no Meta, a multi-value parameter, and a parameter of type
// "record" or "recordset" have no lookup sources. A collection whose schema
// is unknown is not a source.
//
// Order is deterministic: sources in hostSource first (pass "" when the host
// has no source of its own), then by Source, then by Collection.
//
// Pure: no I/O, no mutation of its inputs.
func LookupSources(entities []*datatug.Entity, schemaByCollection map[SchemaKey]TableSchema, parameter datatug.ParameterDef, hostSource string) []LookupSource {
	if parameter.Meta == nil || parameter.IsMultiValue {
		return nil
	}
	var sources []LookupSource
	for _, e := range entities {
		if e == nil || e.ID != parameter.Meta.Entity {
			continue
		}
		for _, f := range e.Fields {
			if f == nil || f.ID != parameter.Meta.Field {
				continue
			}
			for _, ref := range f.Mappings {
				schema, ok := schemaByCollection[SchemaKey{Source: ref.Source, Collection: ref.Collection}]
				if !ok || schema.PrimaryKey == nil || len(schema.PrimaryKey.Columns) != 1 || schema.PrimaryKey.Columns[0] != ref.Column {
					continue
				}
				kind := keyKind(schema, ref.Column, f.Type)
				if !ParameterAcceptsKey(parameter.Type, kind) {
					continue
				}
				display, provenance := DisplayColumns(e, schema, ref)
				sources = append(sources, LookupSource{
					Source: ref.Source, Collection: ref.Collection, KeyColumn: ref.Column, KeyKind: kind,
					Display: display, DisplayProvenance: provenance,
				})
			}
		}
	}
	sort.SliceStable(sources, func(i, j int) bool {
		hi, hj := sources[i].Source == hostSource, sources[j].Source == hostSource
		if hi != hj {
			return hi
		}
		if sources[i].Source != sources[j].Source {
			return sources[i].Source < sources[j].Source
		}
		return sources[i].Collection < sources[j].Collection
	})
	return sources
}

// keyKind is the kind of the key column: from the schema's own column list
// when it has one (a key column missing from it is no key), otherwise from
// the entity field's declared type.
func keyKind(schema TableSchema, keyColumn, fieldType string) KeyKind {
	if len(schema.Columns) == 0 {
		return ColumnKeyKind(fieldType)
	}
	for _, c := range schema.Columns {
		if c.Name == keyColumn {
			return ColumnKeyKind(c.Type)
		}
	}
	return ""
}

// DisplayColumns returns the columns a lookup picker shows beside the key of
// the collection key names (backstage parameter-lookup REQ:display-columns).
//
// Declared: each field ID in entity.Display, in order, that has a declared
// mapping in key's source and collection becomes a column titled with the
// field's Title (or its ID); at most six. When that yields nothing, up to three
// columns are inferred: first the entity's other fields mapped in the same
// collection (field order), then the schema's first text columns (schema
// order). The key column is never repeated, and when the schema lists its
// columns every display column must be one of them. entity must not be nil.
func DisplayColumns(entity *datatug.Entity, schema TableSchema, key datatug.PhysicalRef) ([]DisplayColumn, Provenance) {
	chosen := map[string]bool{key.Column: true}
	known := func(column string) bool {
		if len(schema.Columns) == 0 {
			return true
		}
		for _, c := range schema.Columns {
			if c.Name == column {
				return true
			}
		}
		return false
	}
	columnOf := func(f *datatug.EntityField) (string, bool) {
		for _, ref := range f.Mappings {
			if ref.Source == key.Source && ref.Collection == key.Collection {
				return ref.Column, !chosen[ref.Column] && known(ref.Column)
			}
		}
		return "", false
	}
	label := func(f *datatug.EntityField) string {
		if f.Title != "" {
			return f.Title
		}
		return f.ID
	}

	var declared []DisplayColumn
	for _, id := range entity.Display {
		if len(declared) == maxDeclaredDisplay {
			break
		}
		for _, f := range entity.Fields {
			if f == nil || f.ID != id {
				continue
			}
			if column, ok := columnOf(f); ok {
				chosen[column] = true
				declared = append(declared, DisplayColumn{Column: column, Field: f.ID, Label: label(f), Provenance: Declared})
			}
			break
		}
	}
	if len(declared) > 0 {
		return declared, Declared
	}

	var inferred []DisplayColumn
	for _, f := range entity.Fields {
		if len(inferred) == maxInferredDisplay {
			return inferred, Inferred
		}
		if f == nil {
			continue
		}
		if column, ok := columnOf(f); ok {
			chosen[column] = true
			inferred = append(inferred, DisplayColumn{Column: column, Field: f.ID, Label: label(f), Provenance: Inferred})
		}
	}
	for _, c := range schema.Columns {
		if len(inferred) == maxInferredDisplay {
			break
		}
		if chosen[c.Name] || ColumnKeyKind(c.Type) != KeyText {
			continue
		}
		chosen[c.Name] = true
		inferred = append(inferred, DisplayColumn{Column: c.Name, Label: c.Name, Provenance: Inferred})
	}
	if len(inferred) == 0 {
		return nil, Declared
	}
	return inferred, Inferred
}
