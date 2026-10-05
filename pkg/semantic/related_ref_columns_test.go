package semantic

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

// schemaWithKey is a source of two collections: Line holds fk, and Order has
// the primary key refPK (nil: its schema is not known to the caller).
func schemaWithKey(fk *datatug.ForeignKey, refPK *datatug.UniqueKey, withRefSchema bool) map[SchemaKey]TableSchema {
	schema := map[SchemaKey]TableSchema{
		{Source: "shop", Collection: "Line"}: {ForeignKeys: datatug.ForeignKeys{fk}},
	}
	if withRefSchema {
		schema[SchemaKey{Source: "shop", Collection: "Order"}] = TableSchema{PrimaryKey: refPK}
	}
	return schema
}

func lineValue(column string) SemanticValue {
	return SemanticValue{Source: "shop", Collection: "Line", Column: column}
}

func TestRelatedLookups_ForeignKeyUsesReferencedColumns(t *testing.T) {
	order := datatug.NewTableKey("Order", "", "", nil)

	t.Run("single_column_not_the_primary_key", func(t *testing.T) {
		// Line.OrderNo points to Order.Number, which is not Order's primary key
		// (Id): the guess would be Id, the key says Number.
		fk := &datatug.ForeignKey{Name: "FK_Line_Order", Columns: []string{"OrderNo"}, RefColumns: []string{"Number"}, RefTable: order}
		got := RelatedLookups(nil, schemaWithKey(fk, &datatug.UniqueKey{Name: "PK", Columns: []string{"Id"}}, true), lineValue("OrderNo"))
		assert.Equal(t, []Lookup{{Source: "shop", Collection: "Order", Column: "Number", Kind: LookupForeignKey, Via: "FK_Line_Order"}}, got)
	})

	t.Run("referenced_schema_unknown", func(t *testing.T) {
		fk := &datatug.ForeignKey{Name: "FK_Line_Order", Columns: []string{"OrderNo"}, RefColumns: []string{"Number"}, RefTable: order}
		got := RelatedLookups(nil, schemaWithKey(fk, nil, false), lineValue("OrderNo"))
		assert.Equal(t, []Lookup{{Source: "shop", Collection: "Order", Column: "Number", Kind: LookupForeignKey, Via: "FK_Line_Order"}}, got)
	})

	t.Run("composite_each_column_to_its_own", func(t *testing.T) {
		// The guess for a composite key was the selected column's own name.
		fk := &datatug.ForeignKey{Name: "FK_Line_Order", Columns: []string{"OrderYear", "OrderNo"}, RefColumns: []string{"Year", "Number"}, RefTable: order}
		schema := schemaWithKey(fk, &datatug.UniqueKey{Name: "PK", Columns: []string{"Year", "Number"}}, true)
		assert.Equal(t, "Year", RelatedLookups(nil, schema, lineValue("OrderYear"))[0].Column)
		assert.Equal(t, "Number", RelatedLookups(nil, schema, lineValue("OrderNo"))[0].Column)
	})

	t.Run("column_listed_twice_takes_the_first", func(t *testing.T) {
		fk := &datatug.ForeignKey{Name: "FK", Columns: []string{"A", "A"}, RefColumns: []string{"X", "Y"}, RefTable: order}
		got := RelatedLookups(nil, schemaWithKey(fk, nil, false), lineValue("A"))
		assert.Equal(t, "X", got[0].Column)
	})
}

func TestRelatedLookups_ForeignKeyWithoutUsableReferencedColumnsKeepsTheGuess(t *testing.T) {
	order := datatug.NewTableKey("Order", "", "", nil)
	pk := &datatug.UniqueKey{Name: "PK", Columns: []string{"Id"}}

	t.Run("none_single_column_primary_key", func(t *testing.T) {
		fk := &datatug.ForeignKey{Name: "FK", Columns: []string{"OrderNo"}, RefTable: order}
		got := RelatedLookups(nil, schemaWithKey(fk, pk, true), lineValue("OrderNo"))
		assert.Equal(t, "Id", got[0].Column)
	})

	t.Run("none_referenced_schema_unknown", func(t *testing.T) {
		fk := &datatug.ForeignKey{Name: "FK", Columns: []string{"OrderNo"}, RefTable: order}
		got := RelatedLookups(nil, schemaWithKey(fk, nil, false), lineValue("OrderNo"))
		assert.Equal(t, "OrderNo", got[0].Column)
	})

	t.Run("count_differs_from_columns", func(t *testing.T) {
		// Not a key Validate accepts; a caller that built one by hand gets the
		// guess, never a column picked from a list that is out of step.
		fk := &datatug.ForeignKey{Name: "FK", Columns: []string{"A", "B"}, RefColumns: []string{"X"}, RefTable: order}
		got := RelatedLookups(nil, schemaWithKey(fk, pk, true), lineValue("B"))
		assert.Equal(t, "Id", got[0].Column)
	})
}

func TestRelatedLookups_ForeignKeyOfAnotherColumnYieldsNothing(t *testing.T) {
	fk := &datatug.ForeignKey{Name: "FK", Columns: []string{"OrderNo"}, RefColumns: []string{"Number"}, RefTable: datatug.NewTableKey("Order", "", "", nil)}
	assert.Empty(t, RelatedLookups(nil, schemaWithKey(fk, nil, false), lineValue("Quantity")))
}
