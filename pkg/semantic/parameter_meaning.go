package semantic

import (
	"sort"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// ParameterMeaning is the meaning a parameter's lookup resolves through
// (backstage parameter-lookup REQ:inferred-parameter-meaning).
type ParameterMeaning struct {
	// Ref is the entity field the parameter means.
	Ref datatug.EntityFieldRef
	// Provenance is Declared when Ref is the parameter's own Meta and
	// Inferred when it came from a key field's NamePatterns.
	Provenance Provenance
	// Rule names the pattern that matched, for an Inferred meaning, in the
	// form semantic.Resolve uses ("namePattern:regexp:\\w+CustomerID").
	Rule string
}

// ResolveParameterMeaning returns the meaning of parameter: its declared
// Meta when set (Declared, even when no entity declares that field), else
// the one key field (EntityField.IsKeyField) whose NamePatterns match the
// parameter's ID without a leading "@" (Inferred). Patterns are matched as
// semantic.Resolve matches column names.
//
// ok is false when the parameter has no Meta and no key field matches, or
// when key fields of more than one entity field match: an ambiguous name
// gives no meaning, never a guess between entities. A pattern that cannot be
// evaluated (an invalid regexp, an unknown type) never matches.
//
// Pure: no I/O, no mutation of its inputs.
func ResolveParameterMeaning(entities []*datatug.Entity, parameter datatug.ParameterDef) (meaning ParameterMeaning, ok bool) {
	if parameter.Meta != nil {
		return ParameterMeaning{Ref: *parameter.Meta, Provenance: Declared}, true
	}
	name := strings.TrimPrefix(parameter.ID, "@")
	if name == "" {
		return ParameterMeaning{}, false
	}
	var matches []ParameterMeaning
	for _, e := range entities {
		if e == nil {
			continue
		}
		for _, f := range e.Fields {
			if f == nil || !f.IsKeyField {
				continue
			}
			for _, p := range f.NamePatterns {
				if matched, rule, err := matchPattern(p, name); err == nil && matched {
					matches = append(matches, ParameterMeaning{
						Ref:        datatug.EntityFieldRef{Entity: e.ID, Field: f.ID},
						Provenance: Inferred,
						Rule:       rule,
					})
					break
				}
			}
		}
	}
	if len(matches) != 1 {
		return ParameterMeaning{}, false
	}
	return matches[0], true
}

// sortSources orders lookup sources: hostSource first, then by Source, then
// by Collection.
func sortSources(sources []LookupSource, hostSource string) {
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
}
