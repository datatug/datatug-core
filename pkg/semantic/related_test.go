package semantic

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func chinookSchema() map[SchemaKey]TableSchema {
	return map[SchemaKey]TableSchema{
		{Source: "chinook", Collection: "Customer"}: {
			PrimaryKey: &datatug.UniqueKey{Name: "PK_Customer", Columns: []string{"CustomerId"}},
			ReferencedBy: datatug.ReferencedBys{
				{
					DBCollectionKey: datatug.NewTableKey("Invoice", "", "", nil),
					ForeignKeys:     []*datatug.RefByForeignKey{{Name: "FK_Invoice_Customer", Columns: []string{"CustomerId"}}},
				},
			},
		},
		{Source: "chinook", Collection: "Invoice"}: {
			PrimaryKey: &datatug.UniqueKey{Name: "PK_Invoice", Columns: []string{"InvoiceId"}},
			ForeignKeys: datatug.ForeignKeys{
				{Name: "FK_Invoice_Customer", Columns: []string{"CustomerId"}, RefTable: datatug.NewTableKey("Customer", "", "", nil)},
			},
		},
	}
}

func customerWithCrossSourceMapping() []*datatug.Entity {
	return []*datatug.Entity{
		{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customer"}},
			Fields: datatug.EntityFields{
				{
					ID:   "id",
					Type: "string",
					Mappings: datatug.PhysicalRefs{
						{Source: "chinook", Collection: "Customer", Column: "CustomerId"},
						{Source: "support-notes", Collection: "Customer", Column: "CustomerId"},
					},
				},
			},
		},
	}
}

func TestRelatedLookups_ReferencedBy(t *testing.T) {
	selected := SemanticValue{Entity: "customer", Field: "id", Value: 5, Source: "chinook", Collection: "Customer", Column: "CustomerId", Provenance: Declared}
	got := RelatedLookups(nil, chinookSchema(), selected)
	assert.Len(t, got, 1)
	assert.Equal(t, Lookup{Source: "chinook", Collection: "Invoice", Column: "CustomerId", Kind: LookupReferencedBy, Via: "FK_Invoice_Customer"}, got[0])
}

func TestRelatedLookups_ForeignKey(t *testing.T) {
	selected := SemanticValue{Entity: "invoice", Field: "customerId", Value: 5, Source: "chinook", Collection: "Invoice", Column: "CustomerId", Provenance: Declared}
	got := RelatedLookups(nil, chinookSchema(), selected)
	assert.Len(t, got, 1)
	assert.Equal(t, Lookup{Source: "chinook", Collection: "Customer", Column: "CustomerId", Kind: LookupForeignKey, Via: "FK_Invoice_Customer"}, got[0])
}

func TestRelatedLookups_CrossSourceViaMapping(t *testing.T) {
	selected := SemanticValue{Entity: "customer", Field: "id", Value: 5, Source: "chinook", Collection: "Customer", Column: "CustomerId", Provenance: Declared}
	got := RelatedLookups(customerWithCrossSourceMapping(), nil, selected)
	assert.Len(t, got, 1)
	assert.Equal(t, Lookup{Source: "support-notes", Collection: "Customer", Column: "CustomerId", Kind: LookupSameField, Via: "field:customer.id"}, got[0])
}

func TestRelatedLookups_BothDirectionsAndCrossSourceTogether(t *testing.T) {
	selected := SemanticValue{Entity: "customer", Field: "id", Value: 5, Source: "chinook", Collection: "Customer", Column: "CustomerId", Provenance: Declared}
	got := RelatedLookups(customerWithCrossSourceMapping(), chinookSchema(), selected)
	assert.Len(t, got, 2)
	assert.Equal(t, "chinook", got[0].Source, "same-source lookups must come before cross-source ones")
	assert.Equal(t, LookupReferencedBy, got[0].Kind)
	assert.Equal(t, "support-notes", got[1].Source)
	assert.Equal(t, LookupSameField, got[1].Kind)
}

func TestRelatedLookups_NoFKAndNoMapping_Empty(t *testing.T) {
	selected := SemanticValue{Entity: "customer", Field: "email", Value: "a@b.com", Source: "chinook", Collection: "Customer", Column: "Email", Provenance: Inferred}
	got := RelatedLookups(customerWithCrossSourceMapping(), chinookSchema(), selected)
	assert.Empty(t, got, "a column that is neither a FK, part of the PK, nor mapped elsewhere yields nothing")
}

func TestRelatedLookups_UnknownSchemaKey_Empty(t *testing.T) {
	selected := SemanticValue{Entity: "customer", Field: "id", Value: 5, Source: "other", Collection: "Customer", Column: "CustomerId"}
	got := RelatedLookups(nil, chinookSchema(), selected)
	assert.Empty(t, got)
}

func TestRelatedLookups_DeterministicOrdering(t *testing.T) {
	schema := map[SchemaKey]TableSchema{
		{Source: "chinook", Collection: "Customer"}: {
			PrimaryKey: &datatug.UniqueKey{Name: "PK_Customer", Columns: []string{"CustomerId"}},
			ReferencedBy: datatug.ReferencedBys{
				{
					DBCollectionKey: datatug.NewTableKey("Invoice", "", "", nil),
					ForeignKeys:     []*datatug.RefByForeignKey{{Name: "FK_Invoice_Customer", Columns: []string{"CustomerId"}}},
				},
				{
					DBCollectionKey: datatug.NewTableKey("Bookmark", "", "", nil),
					ForeignKeys:     []*datatug.RefByForeignKey{{Name: "FK_Bookmark_Customer", Columns: []string{"CustomerId"}}},
				},
			},
		},
	}
	entities := []*datatug.Entity{
		{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customer"}},
			Fields: datatug.EntityFields{
				{ID: "id", Type: "string", Mappings: datatug.PhysicalRefs{
					{Source: "chinook", Collection: "Customer", Column: "CustomerId"},
					{Source: "zzz-source", Collection: "Customer", Column: "CustomerId"},
					{Source: "aaa-source", Collection: "Customer", Column: "CustomerId"},
				}},
			},
		},
	}
	selected := SemanticValue{Entity: "customer", Field: "id", Value: 5, Source: "chinook", Collection: "Customer", Column: "CustomerId"}
	got := RelatedLookups(entities, schema, selected)
	assert.Len(t, got, 4)
	assert.Equal(t, "Bookmark", got[0].Collection, "same-source lookups ordered by collection name")
	assert.Equal(t, "Invoice", got[1].Collection)
	assert.Equal(t, "aaa-source", got[2].Source, "cross-source lookups ordered by source name")
	assert.Equal(t, "zzz-source", got[3].Source)
}

func TestRelatedLookups_SameCollectionTieBrokenByKind(t *testing.T) {
	schema := map[SchemaKey]TableSchema{
		{Source: "chinook", Collection: "Self"}: {
			PrimaryKey: &datatug.UniqueKey{Name: "PK_Self", Columns: []string{"Id"}},
			ForeignKeys: datatug.ForeignKeys{
				{Name: "FK_Self_Self", Columns: []string{"Id"}, RefTable: datatug.NewTableKey("Self", "", "", nil)},
			},
			ReferencedBy: datatug.ReferencedBys{
				{
					DBCollectionKey: datatug.NewTableKey("Self", "", "", nil),
					ForeignKeys:     []*datatug.RefByForeignKey{{Name: "FK_Other_Self", Columns: []string{"Id"}}},
				},
			},
		},
	}
	selected := SemanticValue{Entity: "self", Field: "id", Source: "chinook", Collection: "Self", Column: "Id"}
	got := RelatedLookups(nil, schema, selected)
	assert.Len(t, got, 2)
	assert.Equal(t, LookupForeignKey, got[0].Kind, "same collection name ties are broken by Kind")
	assert.Equal(t, LookupReferencedBy, got[1].Kind)
}

func TestRelatedLookups_SameSourceTieBrokenByCollection(t *testing.T) {
	entity := &datatug.Entity{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customer"}},
		Fields: datatug.EntityFields{
			{ID: "id", Type: "string", Mappings: datatug.PhysicalRefs{
				{Source: "chinook", Collection: "Customer", Column: "CustomerId"},
				{Source: "other-source", Collection: "TableB", Column: "X"},
				{Source: "other-source", Collection: "TableA", Column: "Y"},
			}},
		},
	}
	selected := SemanticValue{Entity: "customer", Field: "id", Source: "chinook", Collection: "Customer", Column: "CustomerId"}
	got := RelatedLookups([]*datatug.Entity{entity}, nil, selected)
	assert.Len(t, got, 2)
	assert.Equal(t, "TableA", got[0].Collection, "same source name ties are broken by Collection")
	assert.Equal(t, "TableB", got[1].Collection)
}

func TestRelatedLookups_EmptyInputs(t *testing.T) {
	assert.Empty(t, RelatedLookups(nil, nil, SemanticValue{}))
}

func TestRelatedLookups_NilEntriesSkipped(t *testing.T) {
	entities := []*datatug.Entity{nil, {
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customer"}},
		Fields: datatug.EntityFields{nil, {
			ID: "id", Type: "string",
			Mappings: datatug.PhysicalRefs{{Source: "support-notes", Collection: "Customer", Column: "CustomerId"}},
		}},
	}}
	schema := map[SchemaKey]TableSchema{
		{Source: "chinook", Collection: "Customer"}: {
			ReferencedBy: datatug.ReferencedBys{nil, {
				DBCollectionKey: datatug.NewTableKey("Invoice", "", "", nil),
				ForeignKeys:     []*datatug.RefByForeignKey{nil, {Name: "FK_Invoice_Customer", Columns: []string{"CustomerId"}}},
			}},
			ForeignKeys: datatug.ForeignKeys{nil},
			PrimaryKey:  &datatug.UniqueKey{Name: "PK_Customer", Columns: []string{"CustomerId"}},
		},
	}
	selected := SemanticValue{Entity: "customer", Field: "id", Source: "chinook", Collection: "Customer", Column: "CustomerId"}
	got := RelatedLookups(entities, schema, selected)
	assert.Len(t, got, 2)
}
