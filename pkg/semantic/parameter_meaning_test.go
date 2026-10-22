package semantic

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

// keyedEntity is an entity whose key field ID carries patterns and maps to
// the given collection's key column.
func keyedEntity(id, source, collection, column, fieldType string, patterns ...*datatug.StringPattern) *datatug.Entity {
	return &datatug.Entity{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: id}},
		Fields: datatug.EntityFields{
			nil,
			{ID: "ID", Type: fieldType, IsKeyField: true, NamePatterns: patterns, Mappings: datatug.PhysicalRefs{
				{Source: source, Collection: collection, Column: column},
			}},
		},
	}
}

func regexpPattern(value string) *datatug.StringPattern {
	return &datatug.StringPattern{Type: "regexp", Value: value, CaseSensitive: true}
}

func TestResolveParameterMeaning(t *testing.T) {
	customer := keyedEntity("Customer", "northwind", "Customers", "CustomerID", "string", regexpPattern(`^\w*CustomerID$`))

	t.Run("declared meta wins over a matching pattern", func(t *testing.T) {
		meta := &datatug.EntityFieldRef{Entity: "Other", Field: "Key"}
		got, ok := ResolveParameterMeaning([]*datatug.Entity{customer}, datatug.ParameterDef{ID: "ShipCustomerID", Meta: meta})
		assert.True(t, ok)
		assert.Equal(t, ParameterMeaning{Ref: *meta, Provenance: Declared}, got)
	})

	t.Run("name matches one key field, @ stripped", func(t *testing.T) {
		got, ok := ResolveParameterMeaning([]*datatug.Entity{nil, customer}, datatug.ParameterDef{ID: "@ShipCustomerID"})
		assert.True(t, ok)
		assert.Equal(t, ParameterMeaning{
			Ref:        datatug.EntityFieldRef{Entity: "Customer", Field: "ID"},
			Provenance: Inferred,
			Rule:       `namePattern:regexp:^\w*CustomerID$`,
		}, got)
	})

	t.Run("no match", func(t *testing.T) {
		_, ok := ResolveParameterMeaning([]*datatug.Entity{customer}, datatug.ParameterDef{ID: "Region"})
		assert.False(t, ok)
	})

	t.Run("empty name", func(t *testing.T) {
		_, ok := ResolveParameterMeaning([]*datatug.Entity{customer}, datatug.ParameterDef{ID: "@"})
		assert.False(t, ok)
	})

	t.Run("only key fields count", func(t *testing.T) {
		notKey := keyedEntity("Customer", "northwind", "Customers", "CustomerID", "string", regexpPattern(`CustomerID`))
		notKey.Fields[1].IsKeyField = false
		_, ok := ResolveParameterMeaning([]*datatug.Entity{notKey}, datatug.ParameterDef{ID: "CustomerID"})
		assert.False(t, ok)
	})

	t.Run("more than one entity matches: ambiguous", func(t *testing.T) {
		chinook := keyedEntity("ChinookCustomer", "chinook", "Customer", "CustomerId", "integer",
			&datatug.StringPattern{Type: "exact", Value: "customerid"})
		_, ok := ResolveParameterMeaning([]*datatug.Entity{customer, chinook}, datatug.ParameterDef{ID: "CustomerID"})
		assert.False(t, ok)
	})

	t.Run("two patterns of one field count once", func(t *testing.T) {
		twice := keyedEntity("Customer", "northwind", "Customers", "CustomerID", "string",
			&datatug.StringPattern{Type: "exact", Value: "CustomerID", CaseSensitive: true}, regexpPattern(`CustomerID`))
		got, ok := ResolveParameterMeaning([]*datatug.Entity{twice}, datatug.ParameterDef{ID: "CustomerID"})
		assert.True(t, ok)
		assert.Equal(t, "namePattern:exact:CustomerID", got.Rule)
	})

	t.Run("invalid or nil patterns never match", func(t *testing.T) {
		broken := keyedEntity("Customer", "northwind", "Customers", "CustomerID", "string",
			nil, regexpPattern(`(`), &datatug.StringPattern{Type: "glob", Value: "*"})
		_, ok := ResolveParameterMeaning([]*datatug.Entity{broken}, datatug.ParameterDef{ID: "CustomerID"})
		assert.False(t, ok)
	})
}

func TestLookupSources_InferredMeaning(t *testing.T) {
	customer := keyedEntity("Customer", "northwind", "Customers", "CustomerID", "string", regexpPattern(`\w+CustomerID`))
	schemas := map[SchemaKey]TableSchema{{Source: "northwind", Collection: "Customers"}: northwindCustomersSchema}

	t.Run("a meta-less parameter resolves through its name", func(t *testing.T) {
		sources := LookupSources([]*datatug.Entity{customer}, schemas, datatug.ParameterDef{ID: "ShipCustomerID", Type: "string"}, "")
		if assert.Len(t, sources, 1) {
			assert.Equal(t, "CustomerID", sources[0].KeyColumn)
			assert.Equal(t, ParameterMeaning{
				Ref:        datatug.EntityFieldRef{Entity: "Customer", Field: "ID"},
				Provenance: Inferred,
				Rule:       `namePattern:regexp:\w+CustomerID`,
			}, sources[0].Meaning)
		}
	})

	t.Run("an ambiguous name has no sources", func(t *testing.T) {
		other := keyedEntity("Supplier", "northwind", "Suppliers", "SupplierID", "string", regexpPattern(`Ship\w+`))
		assert.Nil(t, LookupSources([]*datatug.Entity{customer, other}, schemas, datatug.ParameterDef{ID: "ShipCustomerID", Type: "string"}, ""))
	})

	t.Run("an inferred meaning still needs a declared single-column key", func(t *testing.T) {
		composite := map[SchemaKey]TableSchema{{Source: "northwind", Collection: "Customers"}: {PrimaryKey: pk("CustomerID", "Region")}}
		assert.Nil(t, LookupSources([]*datatug.Entity{customer}, composite, datatug.ParameterDef{ID: "ShipCustomerID", Type: "string"}, ""))
	})
}
