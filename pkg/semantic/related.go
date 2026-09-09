package semantic

import (
	"sort"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// SemanticValue is one resolved cell: a value in a physical column, and the
// semantic field a resolver (see Resolve) said it means. Provenance is a
// deliberate addition beyond the field-mapping-model REQ's own sketch of
// this type: it is needed to render the "(declared)"/"(inferred)" segment of
// an Applicable resolution Chain (REQ:applicable-queries), so a caller can
// build one SemanticValue directly from a Resolution and pass it to both
// RelatedLookups and Applicable.
type SemanticValue struct {
	Entity     string
	Field      string
	Value      interface{}
	Source     string
	Collection string
	Column     string
	Provenance Provenance
}

// SchemaKey identifies one collection (table or view) within one source.
type SchemaKey struct {
	Source     string
	Collection string
}

// TableSchema is the subset of a scanned table/view's metadata RelatedLookups
// needs: its primary key, and its foreign-key graph in both directions
// (datatug.ForeignKeys/datatug.ReferencedBys, as scanned by the DB driver).
type TableSchema struct {
	PrimaryKey   *datatug.UniqueKey
	ForeignKeys  datatug.ForeignKeys
	ReferencedBy datatug.ReferencedBys
}

// LookupKind identifies how a Lookup was derived.
type LookupKind string

const (
	// LookupForeignKey means the selected value's own column is a foreign
	// key, pointing at Lookup.Collection.
	LookupForeignKey LookupKind = "foreignKey"
	// LookupReferencedBy means Lookup.Collection has a foreign key pointing
	// back at the selected value's table.
	LookupReferencedBy LookupKind = "referencedBy"
	// LookupSameField means Lookup.Collection maps the same semantic field
	// (the selected value's Entity + Field) in a different source or
	// collection.
	LookupSameField LookupKind = "sameField"
)

// Lookup is one related-record path reachable from a SemanticValue: the
// caller filters Lookup.Collection (in Lookup.Source) where Lookup.Column
// equals the selected value - RelatedLookups itself performs no such filter,
// it only says where to look (REQ:related-lookup-execution builds and runs
// that query server-side).
type Lookup struct {
	Source     string
	Collection string
	Column     string
	Kind       LookupKind
	Via        string
}

// RelatedLookups returns every other collection reachable from a selected
// semantic value: through declared foreign keys in both directions within
// selected's own source (schemaByCollection must carry that source's scanned
// schema for this to find anything), and through mappings of the same
// entity field (selected.Entity + selected.Field) in other sources or
// collections. A column that is neither part of a foreign-key relationship
// nor mapped anywhere else yields nothing.
//
// The scanned-schema model does not record which column of a referenced
// table an outgoing foreign key targets, only the referencing table's own
// columns (datatug.ForeignKey has no "referenced column" field). Two
// heuristics fill that gap, both using the referenced table's primary key as
// the assumed join target - the common case, and the only information this
// model actually carries:
//   - LookupForeignKey: selected.Column matches one of the current table's own
//     ForeignKey.Columns: this reports RefTable's primary key (its first
//     column, when the schema for RefTable is known and single-column-keyed;
//     the selected column's own name otherwise) as the join column.
//   - LookupReferencedBy: selected.Column must be part of the current table's
//     own primary key for a "referenced by" lookup to fire at all, since
//     that is this model's only signal that an incoming foreign key targets
//     this exact column.
//
// Order is deterministic: lookups in selected's own source come first,
// ordered by target Collection name (ties broken by Kind); then cross-source
// SameField lookups, ordered by Source then Collection name. Pure: no I/O.
func RelatedLookups(entities []*datatug.Entity, schemaByCollection map[SchemaKey]TableSchema, selected SemanticValue) []Lookup {
	var sameSource []Lookup
	if schema, ok := schemaByCollection[SchemaKey{Source: selected.Source, Collection: selected.Collection}]; ok {
		sameSource = append(sameSource, foreignKeyLookups(schema, schemaByCollection, selected)...)
		sameSource = append(sameSource, referencedByLookups(schema, selected)...)
	}
	sort.Slice(sameSource, func(i, j int) bool {
		if sameSource[i].Collection != sameSource[j].Collection {
			return sameSource[i].Collection < sameSource[j].Collection
		}
		return sameSource[i].Kind < sameSource[j].Kind
	})

	crossSource := sameFieldLookups(entities, selected)
	sort.Slice(crossSource, func(i, j int) bool {
		if crossSource[i].Source != crossSource[j].Source {
			return crossSource[i].Source < crossSource[j].Source
		}
		return crossSource[i].Collection < crossSource[j].Collection
	})

	return append(sameSource, crossSource...)
}

func foreignKeyLookups(schema TableSchema, schemaByCollection map[SchemaKey]TableSchema, selected SemanticValue) []Lookup {
	var lookups []Lookup
	for _, fk := range schema.ForeignKeys {
		if fk == nil || !containsString(fk.Columns, selected.Column) {
			continue
		}
		refCollection := fk.RefTable.Name()
		refColumn := selected.Column
		if refSchema, ok := schemaByCollection[SchemaKey{Source: selected.Source, Collection: refCollection}]; ok &&
			refSchema.PrimaryKey != nil && len(refSchema.PrimaryKey.Columns) == 1 {
			refColumn = refSchema.PrimaryKey.Columns[0]
		}
		lookups = append(lookups, Lookup{
			Source: selected.Source, Collection: refCollection, Column: refColumn,
			Kind: LookupForeignKey, Via: fk.Name,
		})
	}
	return lookups
}

func referencedByLookups(schema TableSchema, selected SemanticValue) []Lookup {
	if schema.PrimaryKey == nil || !containsString(schema.PrimaryKey.Columns, selected.Column) {
		return nil
	}
	var lookups []Lookup
	for _, rb := range schema.ReferencedBy {
		if rb == nil {
			continue
		}
		for _, fk := range rb.ForeignKeys {
			if fk == nil || len(fk.Columns) == 0 {
				continue
			}
			lookups = append(lookups, Lookup{
				Source: selected.Source, Collection: rb.Name(), Column: strings.Join(fk.Columns, ","),
				Kind: LookupReferencedBy, Via: fk.Name,
			})
		}
	}
	return lookups
}

func sameFieldLookups(entities []*datatug.Entity, selected SemanticValue) []Lookup {
	var lookups []Lookup
	for _, e := range entities {
		if e == nil || e.ID != selected.Entity {
			continue
		}
		for _, f := range e.Fields {
			if f == nil || f.ID != selected.Field {
				continue
			}
			for _, ref := range f.Mappings {
				if ref.Source == selected.Source && ref.Collection == selected.Collection {
					continue // the mapping selected came from is not "related" to itself
				}
				lookups = append(lookups, Lookup{
					Source: ref.Source, Collection: ref.Collection, Column: ref.Column,
					Kind: LookupSameField, Via: "field:" + e.ID + "." + f.ID,
				})
			}
		}
	}
	return lookups
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
