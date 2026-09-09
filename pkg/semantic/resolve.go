package semantic

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Provenance identifies how a Resolution was determined.
type Provenance string

const (
	// Declared means the column matched an EntityField's declared Mappings.
	Declared Provenance = "declared"
	// Inferred means the column matched an EntityField's NamePatterns, with
	// no declared mapping present for it.
	Inferred Provenance = "inferred"
)

// Column is a single physical column to resolve against a project's entities.
type Column struct {
	Name string
	Type string
}

// Resolution is what one Column resolved to.
type Resolution struct {
	Column     string
	Entity     string
	Field      string
	Provenance Provenance
	// Rule documents how the winner was chosen: which name pattern matched
	// (for Inferred), and/or a tie-break note when more than one field
	// resolved the same column at the same provenance level.
	Rule string
	// Err is set only when the column had NamePatterns candidates that could
	// not be evaluated (e.g. an invalid regexp) and neither a declared
	// mapping nor a valid inferred candidate resolved it. When Err is set,
	// Entity, Field, Provenance and Rule are zero.
	Err error
}

// Resolve returns, for each Column, the entity field it maps to in the given
// source/collection (table): declared EntityField.Mappings win; absent a
// declared mapping, EntityField.NamePatterns are tried; a column matching
// neither is simply absent from the result (see REQ:field-mapping-model in
// datatug/datatug's core-investigation-loop feature).
//
// When more than one field resolves the same column at the same provenance
// level, the field belonging to the lowest Entity.ID wins (Entity.ID, then
// EntityField.ID, ascending) and the tie is recorded in the winning
// Resolution's Rule. This keeps Resolve deterministic without requiring
// callers to pre-sort entities.
//
// Resolve is pure: it performs no I/O and does not mutate entities. It never
// panics - a pattern that fails to evaluate (e.g. invalid regexp syntax)
// yields a Resolution with Err set instead, unless a declared mapping or
// another, valid pattern already resolved the column.
func Resolve(entities []*datatug.Entity, source, collection string, columns []Column) []Resolution {
	var resolutions []Resolution
	for _, col := range columns {
		if r, ok := resolveColumn(entities, source, collection, col); ok {
			resolutions = append(resolutions, r)
		}
	}
	return resolutions
}

// candidate is one field that resolved a column, pending tie-break.
type candidate struct {
	entityID string
	fieldID  string
	rule     string
}

func resolveColumn(entities []*datatug.Entity, source, collection string, col Column) (Resolution, bool) {
	var declared []candidate
	for _, e := range entities {
		if e == nil {
			continue
		}
		for _, f := range e.Fields {
			if f == nil {
				continue
			}
			for _, ref := range f.Mappings {
				if ref.Source == source && ref.Collection == collection && ref.Column == col.Name {
					declared = append(declared, candidate{entityID: e.ID, fieldID: f.ID})
					break
				}
			}
		}
	}
	if len(declared) > 0 {
		return pickCandidate(declared, col.Name, Declared), true
	}

	var inferred []candidate
	var patternErrs []error
	for _, e := range entities {
		if e == nil {
			continue
		}
		for _, f := range e.Fields {
			if f == nil {
				continue
			}
			for _, p := range f.NamePatterns {
				matched, rule, err := matchPattern(p, col.Name)
				if err != nil {
					patternErrs = append(patternErrs, fmt.Errorf("entity %s field %s: %w", e.ID, f.ID, err))
					continue
				}
				if matched {
					inferred = append(inferred, candidate{entityID: e.ID, fieldID: f.ID, rule: rule})
					break
				}
			}
		}
	}
	if len(inferred) > 0 {
		return pickCandidate(inferred, col.Name, Inferred), true
	}
	if len(patternErrs) > 0 {
		return Resolution{Column: col.Name, Err: joinErrors(patternErrs)}, true
	}
	return Resolution{}, false
}

// pickCandidate deterministically picks a winner among candidates that all
// resolved the same column at the same provenance, tie-breaking on the
// lowest Entity.ID then EntityField.ID and noting the tie in Rule.
func pickCandidate(candidates []candidate, columnName string, provenance Provenance) Resolution {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].entityID != candidates[j].entityID {
			return candidates[i].entityID < candidates[j].entityID
		}
		return candidates[i].fieldID < candidates[j].fieldID
	})
	winner := candidates[0]
	rule := winner.rule
	if len(candidates) > 1 {
		others := make([]string, 0, len(candidates)-1)
		for _, c := range candidates[1:] {
			others = append(others, c.entityID+"."+c.fieldID)
		}
		tieNote := fmt.Sprintf("tie-break: %d %s matches for %q, chose %s.%s (lowest entity id wins); also matched: %s",
			len(candidates), provenance, columnName, winner.entityID, winner.fieldID, strings.Join(others, ", "))
		if rule != "" {
			rule = rule + "; " + tieNote
		} else {
			rule = tieNote
		}
	}
	return Resolution{Column: columnName, Entity: winner.entityID, Field: winner.fieldID, Provenance: provenance, Rule: rule}
}

// matchPattern reports whether columnName matches p, and a Rule note
// describing the match. It never panics: an invalid regexp is returned as an
// error, not compiled with MustCompile.
func matchPattern(p *datatug.StringPattern, columnName string) (matched bool, rule string, err error) {
	if p == nil {
		return false, "", nil
	}
	switch p.Type {
	case "exact":
		if p.CaseSensitive {
			matched = p.Value == columnName
		} else {
			matched = strings.EqualFold(p.Value, columnName)
		}
		return matched, "namePattern:exact:" + p.Value, nil
	case "regexp":
		pattern := p.Value
		if !p.CaseSensitive {
			pattern = "(?i)" + pattern
		}
		re, compileErr := regexp.Compile(pattern)
		if compileErr != nil {
			return false, "", fmt.Errorf("invalid regexp pattern %q: %w", p.Value, compileErr)
		}
		return re.MatchString(columnName), "namePattern:regexp:" + p.Value, nil
	default:
		return false, "", fmt.Errorf("unknown name pattern type %q", p.Type)
	}
}

// joinErrors combines multiple pattern errors into one, without pulling in
// errors.Join's Go-1.20 formatting quirks for a single-error slice.
func joinErrors(errs []error) error {
	if len(errs) == 1 {
		return errs[0]
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return fmt.Errorf("%d name pattern errors: %s", len(errs), strings.Join(msgs, "; "))
}
