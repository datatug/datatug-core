package semantic

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func queryWithMetaParam(id string, entity, field string, required bool) *datatug.QueryDef {
	return &datatug.QueryDef{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: id, Title: id}},
		Type:        datatug.QueryTypeSQL,
		Parameters: datatug.Parameters{
			{ID: "customerId", Type: "integer", IsRequired: required, Meta: &datatug.EntityFieldRef{Entity: entity, Field: field}},
		},
	}
}

func TestApplicable_Satisfiable(t *testing.T) {
	q := queryWithMetaParam("customer-invoices", "Customer", "ID", true)
	available := []SemanticValue{
		{Entity: "Customer", Field: "ID", Value: 5, Source: "chinook", Collection: "Customer", Column: "CustomerId", Provenance: Declared},
	}
	applicable, notYet := Applicable([]*datatug.QueryDef{q}, available)
	assert.Empty(t, notYet)
	assert.Len(t, applicable, 1)
	a := applicable[0]
	assert.Same(t, q, a.Query)
	assert.Len(t, a.Bindings, 1)
	assert.Equal(t, "customerId", a.Bindings[0].Parameter)
	assert.Equal(t, 5, a.Bindings[0].Value)
	assert.Equal(t, []string{"CustomerId → Customer.ID (declared) → parameter customerId"}, a.Chain)
}

func TestApplicable_MissingRequiredParameter(t *testing.T) {
	q := queryWithMetaParam("invoice-lines", "Invoice", "ID", true)
	applicable, notYet := Applicable([]*datatug.QueryDef{q}, nil)
	assert.Empty(t, applicable)
	assert.Len(t, notYet, 1)
	assert.Same(t, q, notYet[0].Query)
	assert.Equal(t, []string{"Invoice.ID"}, notYet[0].Missing)
}

func TestApplicable_OptionalParameterNeverBlocks(t *testing.T) {
	q := queryWithMetaParam("optional-report", "Invoice", "ID", false)
	applicable, notYet := Applicable([]*datatug.QueryDef{q}, nil)
	assert.Empty(t, notYet)
	assert.Len(t, applicable, 1)
	assert.Empty(t, applicable[0].Bindings, "an unsatisfied optional parameter stays unbound, not bound to nothing")
}

func TestApplicable_UntaggedParameterNeverBlocks(t *testing.T) {
	q := &datatug.QueryDef{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q1", Title: "Q1"}},
		Type:        datatug.QueryTypeSQL,
		Parameters: datatug.Parameters{
			{ID: "limit", Type: "integer", IsRequired: true}, // no Meta
		},
	}
	applicable, notYet := Applicable([]*datatug.QueryDef{q}, nil)
	assert.Empty(t, notYet)
	assert.Len(t, applicable, 1)
	assert.Empty(t, applicable[0].Bindings)
	assert.Empty(t, applicable[0].Chain)
}

func TestApplicable_NoParameters(t *testing.T) {
	q := &datatug.QueryDef{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "album-list", Title: "Album list"}},
		Type:        datatug.QueryTypeSQL,
	}
	applicable, notYet := Applicable([]*datatug.QueryDef{q}, nil)
	assert.Empty(t, notYet)
	assert.Len(t, applicable, 1)
	assert.Empty(t, applicable[0].Bindings)
}

func TestApplicable_NewestValueWinsOnDuplicateField(t *testing.T) {
	q := queryWithMetaParam("customer-invoices", "Customer", "ID", true)
	available := []SemanticValue{
		{Entity: "Customer", Field: "ID", Value: 1, Source: "chinook", Collection: "Customer", Column: "CustomerId", Provenance: Declared},
		{Entity: "Customer", Field: "ID", Value: 5, Source: "chinook", Collection: "Customer", Column: "CustomerId", Provenance: Declared},
	}
	applicable, _ := Applicable([]*datatug.QueryDef{q}, available)
	assert.Len(t, applicable, 1)
	assert.Equal(t, 5, applicable[0].Bindings[0].Value, "the later (newest) entry for the same entity+field must win")
}

func TestApplicable_MultipleQueriesMixedOutcomes(t *testing.T) {
	satisfiable := queryWithMetaParam("customer-invoices", "Customer", "ID", true)
	unsatisfiable := queryWithMetaParam("invoice-lines", "Invoice", "ID", true)
	available := []SemanticValue{
		{Entity: "Customer", Field: "ID", Value: 5, Source: "chinook", Collection: "Customer", Column: "CustomerId", Provenance: Declared},
	}
	applicable, notYet := Applicable([]*datatug.QueryDef{satisfiable, unsatisfiable}, available)
	assert.Len(t, applicable, 1)
	assert.Equal(t, "customer-invoices", applicable[0].Query.ID)
	assert.Len(t, notYet, 1)
	assert.Equal(t, "invoice-lines", notYet[0].Query.ID)
}

func TestApplicable_EmptyInputs(t *testing.T) {
	applicable, notYet := Applicable(nil, nil)
	assert.Empty(t, applicable)
	assert.Empty(t, notYet)
}

func TestApplicable_NilQueryInSliceSkipped(t *testing.T) {
	q := queryWithMetaParam("q1", "Customer", "ID", true)
	available := []SemanticValue{{Entity: "Customer", Field: "ID", Value: 5, Source: "chinook", Collection: "Customer", Column: "CustomerId", Provenance: Declared}}
	applicable, notYet := Applicable([]*datatug.QueryDef{nil, q}, available)
	assert.Empty(t, notYet)
	assert.Len(t, applicable, 1)
}

func TestApplicable_InferredProvenanceInChain(t *testing.T) {
	q := queryWithMetaParam("customer-invoices", "Customer", "ID", true)
	available := []SemanticValue{
		{Entity: "Customer", Field: "ID", Value: 5, Source: "chinook", Collection: "Customer", Column: "CustomerId", Provenance: Inferred},
	}
	applicable, _ := Applicable([]*datatug.QueryDef{q}, available)
	assert.Equal(t, []string{"CustomerId → Customer.ID (inferred) → parameter customerId"}, applicable[0].Chain)
}
