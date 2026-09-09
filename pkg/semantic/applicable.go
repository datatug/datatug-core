package semantic

import (
	"fmt"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Binding is one query parameter bound to a semantic value.
type Binding struct {
	Parameter string
	Value     interface{}
	From      SemanticValue
}

// ApplicableQuery is a query every required, semantically-tagged parameter of
// which could be bound from the available values.
type ApplicableQuery struct {
	Query    *datatug.QueryDef
	Bindings []Binding
	// Chain is the human-readable resolution chain, one entry per bound
	// parameter: "<column> → <entity>.<field> (<declared|inferred>) →
	// parameter <parameterID>".
	Chain []string
}

// NotYetApplicable is a query with at least one required, semantically-tagged
// parameter the available values could not satisfy.
type NotYetApplicable struct {
	Query *datatug.QueryDef
	// Missing lists each unsatisfied required parameter's semantic field, as
	// "<entity>.<field>".
	Missing []string
}

// Applicable splits queries into those every required, Meta-tagged parameter
// of which can be bound from available, and those still missing at least
// one. A parameter with no Meta is never considered - it stays unbound and
// never blocks applicability, whether required or not. A Meta-tagged
// optional parameter (IsRequired == false) is bound when available can
// satisfy it, but its absence never blocks applicability either; only an
// unsatisfied *required* Meta-tagged parameter lands a query in notYet.
//
// available is order-sensitive: when more than one entry shares the same
// Entity+Field (e.g. the Investigation Context holds a stale value and the
// user's current selection holds a newer one), the last entry for that
// Entity+Field wins - callers should append newer values after older ones.
//
// Pure: no I/O.
func Applicable(queries []*datatug.QueryDef, available []SemanticValue) (applicable []ApplicableQuery, notYet []NotYetApplicable) {
	byField := indexByEntityField(available)
	for _, q := range queries {
		if q == nil {
			continue
		}
		var bindings []Binding
		var chain []string
		var missing []string
		for _, p := range q.Parameters {
			if p.Meta == nil {
				continue
			}
			sv, ok := byField[entityFieldKey(p.Meta.Entity, p.Meta.Field)]
			if !ok {
				if p.IsRequired {
					missing = append(missing, p.Meta.Entity+"."+p.Meta.Field)
				}
				continue
			}
			bindings = append(bindings, Binding{Parameter: p.ID, Value: sv.Value, From: sv})
			chain = append(chain, fmt.Sprintf("%s → %s.%s (%s) → parameter %s", sv.Column, sv.Entity, sv.Field, sv.Provenance, p.ID))
		}
		if len(missing) > 0 {
			notYet = append(notYet, NotYetApplicable{Query: q, Missing: missing})
			continue
		}
		applicable = append(applicable, ApplicableQuery{Query: q, Bindings: bindings, Chain: chain})
	}
	return applicable, notYet
}

func entityFieldKey(entity, field string) string {
	return entity + "\x00" + field
}

func indexByEntityField(available []SemanticValue) map[string]SemanticValue {
	m := make(map[string]SemanticValue, len(available))
	for _, sv := range available {
		m[entityFieldKey(sv.Entity, sv.Field)] = sv
	}
	return m
}
