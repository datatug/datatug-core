package semantic

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func entityWithMapping(entityID, fieldID, source, collection, column string) *datatug.Entity {
	return &datatug.Entity{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: entityID}},
		Fields: datatug.EntityFields{
			{
				ID:   fieldID,
				Type: "string",
				Mappings: datatug.PhysicalRefs{
					{Source: source, Collection: collection, Column: column},
				},
			},
		},
	}
}

func entityWithPattern(entityID, fieldID string, pattern *datatug.StringPattern) *datatug.Entity {
	return &datatug.Entity{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: entityID}},
		Fields: datatug.EntityFields{
			{
				ID:           fieldID,
				Type:         "string",
				NamePatterns: datatug.StringPatterns{pattern},
			},
		},
	}
}

func TestResolve_EmptyInputs(t *testing.T) {
	t.Run("nil_entities", func(t *testing.T) {
		got := Resolve(nil, "chinook", "Customer", []Column{{Name: "CustomerId"}})
		assert.Empty(t, got)
	})
	t.Run("nil_columns", func(t *testing.T) {
		entities := []*datatug.Entity{entityWithMapping("customer", "id", "chinook", "Customer", "CustomerId")}
		got := Resolve(entities, "chinook", "Customer", nil)
		assert.Empty(t, got)
	})
	t.Run("no_entities_no_columns", func(t *testing.T) {
		got := Resolve(nil, "", "", nil)
		assert.Empty(t, got)
	})
	t.Run("nil_entity_in_slice_is_skipped", func(t *testing.T) {
		entities := []*datatug.Entity{nil, entityWithMapping("customer", "id", "chinook", "Customer", "CustomerId")}
		got := Resolve(entities, "chinook", "Customer", []Column{{Name: "CustomerId"}})
		assert.Len(t, got, 1)
		assert.Equal(t, "customer", got[0].Entity)
	})
}

func TestResolve_NoMatch(t *testing.T) {
	entities := []*datatug.Entity{entityWithMapping("customer", "id", "chinook", "Customer", "CustomerId")}
	got := Resolve(entities, "chinook", "Customer", []Column{{Name: "FirstName"}})
	assert.Empty(t, got, "a column with no declared mapping and no matching pattern must not appear in the result at all")
}

func TestResolve_DeclaredMapping(t *testing.T) {
	entities := []*datatug.Entity{entityWithMapping("customer", "id", "chinook", "Customer", "CustomerId")}
	got := Resolve(entities, "chinook", "Customer", []Column{{Name: "CustomerId", Type: "int"}})
	assert.Len(t, got, 1)
	r := got[0]
	assert.Equal(t, "CustomerId", r.Column)
	assert.Equal(t, "customer", r.Entity)
	assert.Equal(t, "id", r.Field)
	assert.Equal(t, Declared, r.Provenance)
	assert.NoError(t, r.Err)
}

func TestResolve_DeclaredMapping_WrongSourceOrCollectionDoesNotMatch(t *testing.T) {
	entities := []*datatug.Entity{entityWithMapping("customer", "id", "chinook", "Customer", "CustomerId")}
	t.Run("wrong_source", func(t *testing.T) {
		got := Resolve(entities, "other-source", "Customer", []Column{{Name: "CustomerId"}})
		assert.Empty(t, got)
	})
	t.Run("wrong_collection", func(t *testing.T) {
		got := Resolve(entities, "chinook", "Invoice", []Column{{Name: "CustomerId"}})
		assert.Empty(t, got)
	})
}

func TestResolve_InferredMapping(t *testing.T) {
	entities := []*datatug.Entity{
		entityWithPattern("customer", "id", &datatug.StringPattern{Type: "exact", Value: "CustomerId"}),
	}
	got := Resolve(entities, "chinook", "Customer", []Column{{Name: "CustomerId"}})
	assert.Len(t, got, 1)
	r := got[0]
	assert.Equal(t, "customer", r.Entity)
	assert.Equal(t, "id", r.Field)
	assert.Equal(t, Inferred, r.Provenance)
	assert.NotEmpty(t, r.Rule)
	assert.NoError(t, r.Err)
}

func TestResolve_DeclaredWinsOverInferred(t *testing.T) {
	entities := []*datatug.Entity{
		entityWithPattern("legacy_customer", "cid", &datatug.StringPattern{Type: "exact", Value: "CustomerId"}),
		entityWithMapping("customer", "id", "chinook", "Customer", "CustomerId"),
	}
	got := Resolve(entities, "chinook", "Customer", []Column{{Name: "CustomerId"}})
	assert.Len(t, got, 1)
	r := got[0]
	assert.Equal(t, Declared, r.Provenance)
	assert.Equal(t, "customer", r.Entity)
	assert.Equal(t, "id", r.Field)
}

func TestResolve_TieBreak_MultipleDeclared(t *testing.T) {
	e1 := entityWithMapping("z_entity", "f1", "chinook", "Customer", "CustomerId")
	e2 := entityWithMapping("a_entity", "f1", "chinook", "Customer", "CustomerId")
	got := Resolve([]*datatug.Entity{e1, e2}, "chinook", "Customer", []Column{{Name: "CustomerId"}})
	assert.Len(t, got, 1)
	r := got[0]
	assert.Equal(t, Declared, r.Provenance)
	assert.Equal(t, "a_entity", r.Entity, "lowest entity ID must win a tie")
	assert.NotEmpty(t, r.Rule, "a tie must be documented in Rule")
}

func TestResolve_TieBreak_MultipleInferred(t *testing.T) {
	e1 := entityWithPattern("z_entity", "f1", &datatug.StringPattern{Type: "exact", Value: "CustomerId"})
	e2 := entityWithPattern("a_entity", "f1", &datatug.StringPattern{Type: "exact", Value: "CustomerId"})
	got := Resolve([]*datatug.Entity{e1, e2}, "chinook", "Customer", []Column{{Name: "CustomerId"}})
	assert.Len(t, got, 1)
	r := got[0]
	assert.Equal(t, Inferred, r.Provenance)
	assert.Equal(t, "a_entity", r.Entity, "lowest entity ID must win a tie")
	assert.NotEmpty(t, r.Rule, "a tie must be documented in Rule")
}

func TestResolve_CaseInsensitivePatterns(t *testing.T) {
	t.Run("exact_default_case_insensitive", func(t *testing.T) {
		entities := []*datatug.Entity{
			entityWithPattern("customer", "id", &datatug.StringPattern{Type: "exact", Value: "customerid"}),
		}
		got := Resolve(entities, "chinook", "Customer", []Column{{Name: "CustomerId"}})
		assert.Len(t, got, 1)
		assert.Equal(t, Inferred, got[0].Provenance)
	})
	t.Run("regexp_default_case_insensitive", func(t *testing.T) {
		entities := []*datatug.Entity{
			entityWithPattern("customer", "id", &datatug.StringPattern{Type: "regexp", Value: "^customerid$"}),
		}
		got := Resolve(entities, "chinook", "Customer", []Column{{Name: "CustomerId"}})
		assert.Len(t, got, 1)
		assert.Equal(t, Inferred, got[0].Provenance)
	})
	t.Run("case_sensitive_pattern_does_not_match_different_case", func(t *testing.T) {
		entities := []*datatug.Entity{
			entityWithPattern("customer", "id", &datatug.StringPattern{Type: "exact", Value: "customerid", CaseSensitive: true}),
		}
		got := Resolve(entities, "chinook", "Customer", []Column{{Name: "CustomerId"}})
		assert.Empty(t, got)
	})
	t.Run("case_sensitive_pattern_matches_same_case", func(t *testing.T) {
		entities := []*datatug.Entity{
			entityWithPattern("customer", "id", &datatug.StringPattern{Type: "exact", Value: "CustomerId", CaseSensitive: true}),
		}
		got := Resolve(entities, "chinook", "Customer", []Column{{Name: "CustomerId"}})
		assert.Len(t, got, 1)
	})
}

func TestResolve_InvalidRegexpIsErrorResultNotPanic(t *testing.T) {
	entities := []*datatug.Entity{
		entityWithPattern("customer", "id", &datatug.StringPattern{Type: "regexp", Value: "["}),
	}
	assert.NotPanics(t, func() {
		got := Resolve(entities, "chinook", "Customer", []Column{{Name: "CustomerId"}})
		assert.Len(t, got, 1)
		r := got[0]
		assert.Equal(t, "CustomerId", r.Column)
		assert.Error(t, r.Err)
		assert.Empty(t, r.Entity)
		assert.Empty(t, r.Field)
		assert.Empty(t, string(r.Provenance))
	})
}

func TestResolve_InvalidRegexpDoesNotBlockOtherValidCandidates(t *testing.T) {
	entities := []*datatug.Entity{
		entityWithPattern("broken", "f1", &datatug.StringPattern{Type: "regexp", Value: "["}),
		entityWithPattern("working", "f1", &datatug.StringPattern{Type: "exact", Value: "CustomerId"}),
	}
	got := Resolve(entities, "chinook", "Customer", []Column{{Name: "CustomerId"}})
	assert.Len(t, got, 1)
	r := got[0]
	assert.Equal(t, Inferred, r.Provenance)
	assert.Equal(t, "working", r.Entity)
	assert.NoError(t, r.Err)
}

func TestResolve_NilFieldsSkippedInBothPasses(t *testing.T) {
	entities := []*datatug.Entity{
		nil,
		{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customer"}},
			Fields: datatug.EntityFields{
				nil,
				{ID: "id", Type: "string", NamePatterns: datatug.StringPatterns{
					{Type: "exact", Value: "CustomerId"},
				}},
			},
		},
	}
	got := Resolve(entities, "chinook", "Customer", []Column{{Name: "CustomerId"}})
	assert.Len(t, got, 1)
	assert.Equal(t, Inferred, got[0].Provenance)
	assert.Equal(t, "customer", got[0].Entity)
}

func TestResolve_TieBreak_SameEntityDifferentFields(t *testing.T) {
	entity := &datatug.Entity{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customer"}},
		Fields: datatug.EntityFields{
			{ID: "z_field", Type: "string", Mappings: datatug.PhysicalRefs{
				{Source: "chinook", Collection: "Customer", Column: "CustomerId"},
			}},
			{ID: "a_field", Type: "string", Mappings: datatug.PhysicalRefs{
				{Source: "chinook", Collection: "Customer", Column: "CustomerId"},
			}},
		},
	}
	got := Resolve([]*datatug.Entity{entity}, "chinook", "Customer", []Column{{Name: "CustomerId"}})
	assert.Len(t, got, 1)
	r := got[0]
	assert.Equal(t, "customer", r.Entity)
	assert.Equal(t, "a_field", r.Field, "lowest field ID must win a same-entity tie")
	assert.NotEmpty(t, r.Rule)
}

func TestResolve_UnknownPatternTypeIsErrorResult(t *testing.T) {
	entities := []*datatug.Entity{
		entityWithPattern("customer", "id", &datatug.StringPattern{Type: "unknown", Value: "CustomerId"}),
	}
	got := Resolve(entities, "chinook", "Customer", []Column{{Name: "CustomerId"}})
	assert.Len(t, got, 1)
	assert.Error(t, got[0].Err)
	assert.Empty(t, got[0].Entity)
}

func TestResolve_MultiplePatternErrorsAreJoined(t *testing.T) {
	entities := []*datatug.Entity{
		entityWithPattern("e1", "f1", &datatug.StringPattern{Type: "regexp", Value: "["}),
		entityWithPattern("e2", "f1", &datatug.StringPattern{Type: "regexp", Value: "("}),
	}
	got := Resolve(entities, "chinook", "Customer", []Column{{Name: "CustomerId"}})
	assert.Len(t, got, 1)
	assert.Error(t, got[0].Err)
	assert.Contains(t, got[0].Err.Error(), "2 name pattern errors")
}

func TestResolve_NilPatternInSliceSkipped(t *testing.T) {
	entity := &datatug.Entity{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customer"}},
		Fields: datatug.EntityFields{
			{ID: "id", Type: "string", NamePatterns: datatug.StringPatterns{
				nil,
				{Type: "exact", Value: "CustomerId"},
			}},
		},
	}
	got := Resolve([]*datatug.Entity{entity}, "chinook", "Customer", []Column{{Name: "CustomerId"}})
	assert.Len(t, got, 1)
	assert.Equal(t, Inferred, got[0].Provenance)
}

func TestResolve_MultipleColumnsMixedOutcomes(t *testing.T) {
	customer := entityWithMapping("customer", "id", "chinook", "Customer", "CustomerId")
	customer.Fields = append(customer.Fields, &datatug.EntityField{
		ID:           "email",
		Type:         "string",
		NamePatterns: datatug.StringPatterns{{Type: "exact", Value: "Email"}},
	})
	entities := []*datatug.Entity{customer}

	got := Resolve(entities, "chinook", "Customer", []Column{
		{Name: "CustomerId"},
		{Name: "Email"},
		{Name: "FirstName"},
	})
	assert.Len(t, got, 2)
	byColumn := map[string]Resolution{}
	for _, r := range got {
		byColumn[r.Column] = r
	}
	assert.Equal(t, Declared, byColumn["CustomerId"].Provenance)
	assert.Equal(t, Inferred, byColumn["Email"].Provenance)
	_, hasFirstName := byColumn["FirstName"]
	assert.False(t, hasFirstName)
}
