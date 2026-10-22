package semantic

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func pk(columns ...string) *datatug.UniqueKey {
	return &datatug.UniqueKey{Name: "PK", Columns: columns}
}

func northwindCustomer(display ...string) *datatug.Entity {
	return &datatug.Entity{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "Customer"}},
		Display:     display,
		Fields: datatug.EntityFields{
			nil,
			{ID: "ID", Type: "string", Mappings: datatug.PhysicalRefs{
				{Source: "northwind", Collection: "Customers", Column: "CustomerID"},
				{Source: "chinook", Collection: "Customer", Column: "CustomerId"},
			}},
			{ID: "CompanyName", Title: "Company", Type: "string", Mappings: datatug.PhysicalRefs{
				{Source: "northwind", Collection: "Customers", Column: "CompanyName"},
			}},
			{ID: "City", Type: "string", Mappings: datatug.PhysicalRefs{
				{Source: "northwind", Collection: "Customers", Column: "City"},
			}},
		},
	}
}

var northwindCustomersSchema = TableSchema{
	PrimaryKey: pk("CustomerID"),
	Columns: []Column{
		{Name: "CustomerID", Type: "nchar(5)"},
		{Name: "CompanyName", Type: "nvarchar(40)"},
		{Name: "ContactName", Type: "nvarchar(30)"},
		{Name: "ContactTitle", Type: "nvarchar(30)"},
		{Name: "Address", Type: "nvarchar(60)"},
		{Name: "City", Type: "nvarchar(15)"},
	},
}

func customerParam(paramType string) datatug.ParameterDef {
	return datatug.ParameterDef{ID: "CustomerID", Type: paramType, Meta: &datatug.EntityFieldRef{Entity: "Customer", Field: "ID"}}
}

func TestColumnKeyKind(t *testing.T) {
	for columnType, want := range map[string]KeyKind{
		"INTEGER": KeyInteger, "bigint": KeyInteger, " int ": KeyInteger,
		"nchar(5)": KeyText, "VARCHAR(40)": KeyText, "clob": KeyText, "TEXT": KeyText, "string": KeyText,
		"decimal(10,2)": "", "REAL": "", "date": "", "blob": "", "": "",
	} {
		assert.Equal(t, want, ColumnKeyKind(columnType), columnType)
	}
}

func TestParameterAcceptsKey(t *testing.T) {
	assert.True(t, ParameterAcceptsKey("string", KeyText))
	assert.True(t, ParameterAcceptsKey("string", KeyInteger))
	assert.True(t, ParameterAcceptsKey("integer", KeyInteger))
	assert.False(t, ParameterAcceptsKey("integer", KeyText))
	assert.False(t, ParameterAcceptsKey("string", ""))
	assert.False(t, ParameterAcceptsKey("number", KeyInteger))
	assert.False(t, ParameterAcceptsKey("record", KeyText))
}

func TestLookupSources_DeclaredSingleColumnPrimaryKey(t *testing.T) {
	entities := []*datatug.Entity{nil, {ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "Other"}}}, northwindCustomer("CompanyName", "City")}
	schemas := map[SchemaKey]TableSchema{{Source: "northwind", Collection: "Customers"}: northwindCustomersSchema}

	sources := LookupSources(entities, schemas, customerParam("string"), "")

	assert.Equal(t, []LookupSource{{
		Source: "northwind", Collection: "Customers", KeyColumn: "CustomerID", KeyKind: KeyText,
		Display: []DisplayColumn{
			{Column: "CompanyName", Field: "CompanyName", Label: "Company", Provenance: Declared},
			{Column: "City", Field: "City", Label: "City", Provenance: Declared},
		},
		DisplayProvenance: Declared,
	}}, sources, "chinook has no schema, so it is not a source")
}

func TestLookupSources_NoSource(t *testing.T) {
	entities := []*datatug.Entity{northwindCustomer()}
	schemas := map[SchemaKey]TableSchema{{Source: "northwind", Collection: "Customers"}: northwindCustomersSchema}

	t.Run("no meta", func(t *testing.T) {
		assert.Nil(t, LookupSources(entities, schemas, datatug.ParameterDef{ID: "x", Type: "string"}, ""))
	})
	t.Run("multi-value", func(t *testing.T) {
		p := customerParam("string")
		p.IsMultiValue = true
		assert.Nil(t, LookupSources(entities, schemas, p, ""))
	})
	t.Run("record parameter", func(t *testing.T) {
		assert.Nil(t, LookupSources(entities, schemas, customerParam("record"), ""))
	})
	t.Run("text key for integer parameter", func(t *testing.T) {
		assert.Nil(t, LookupSources(entities, schemas, customerParam("integer"), ""))
	})
	t.Run("unknown field", func(t *testing.T) {
		p := customerParam("string")
		p.Meta = &datatug.EntityFieldRef{Entity: "Customer", Field: "Nope"}
		assert.Nil(t, LookupSources(entities, schemas, p, ""))
	})
	t.Run("composite or missing primary key", func(t *testing.T) {
		for _, schema := range []TableSchema{
			{PrimaryKey: pk("CustomerID", "Region")},
			{},
			{PrimaryKey: pk("CompanyName")},
		} {
			got := LookupSources(entities, map[SchemaKey]TableSchema{{Source: "northwind", Collection: "Customers"}: schema}, customerParam("string"), "")
			assert.Nil(t, got)
		}
	})
	t.Run("key column missing from listed columns", func(t *testing.T) {
		schema := TableSchema{PrimaryKey: pk("CustomerID"), Columns: []Column{{Name: "CompanyName", Type: "text"}}}
		assert.Nil(t, LookupSources(entities, map[SchemaKey]TableSchema{{Source: "northwind", Collection: "Customers"}: schema}, customerParam("string"), ""))
	})
	t.Run("unsupported key type", func(t *testing.T) {
		schema := TableSchema{PrimaryKey: pk("CustomerID"), Columns: []Column{{Name: "CustomerID", Type: "decimal"}}}
		assert.Nil(t, LookupSources(entities, map[SchemaKey]TableSchema{{Source: "northwind", Collection: "Customers"}: schema}, customerParam("string"), ""))
	})
	t.Run("name patterns are never sources", func(t *testing.T) {
		inferredOnly := []*datatug.Entity{{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "Customer"}},
			Fields:      datatug.EntityFields{{ID: "ID", Type: "string", NamePatterns: datatug.StringPatterns{{Type: "exact", Value: "CustomerID"}}}},
		}}
		assert.Nil(t, LookupSources(inferredOnly, schemas, customerParam("string"), ""))
	})
}

func TestLookupSources_KeyKindFromFieldTypeWithoutColumns(t *testing.T) {
	entities := []*datatug.Entity{northwindCustomer()}
	schemas := map[SchemaKey]TableSchema{
		{Source: "northwind", Collection: "Customers"}: {PrimaryKey: pk("CustomerID")},
		{Source: "chinook", Collection: "Customer"}:    {PrimaryKey: pk("CustomerId")},
	}
	sources := LookupSources(entities, schemas, customerParam("string"), "")
	assert.Len(t, sources, 2)
	assert.Equal(t, KeyText, sources[0].KeyKind)
	assert.Equal(t, "chinook", sources[0].Source, "by source name without a host source")
	assert.Equal(t, []DisplayColumn{
		{Column: "CompanyName", Field: "CompanyName", Label: "Company", Provenance: Inferred},
		{Column: "City", Field: "City", Label: "City", Provenance: Inferred},
	}, sources[1].Display, "unknown columns are trusted when the schema lists none")
}

func TestLookupSources_Order(t *testing.T) {
	entity := northwindCustomer()
	entity.Fields[1].Mappings = append(entity.Fields[1].Mappings,
		datatug.PhysicalRef{Source: "northwind", Collection: "Accounts", Column: "CustomerID"})
	schemas := map[SchemaKey]TableSchema{
		{Source: "northwind", Collection: "Customers"}: {PrimaryKey: pk("CustomerID")},
		{Source: "northwind", Collection: "Accounts"}:  {PrimaryKey: pk("CustomerID")},
		{Source: "chinook", Collection: "Customer"}:    {PrimaryKey: pk("CustomerId")},
	}
	sources := LookupSources([]*datatug.Entity{entity}, schemas, customerParam("string"), "northwind")
	var got []string
	for _, s := range sources {
		got = append(got, s.Source+"/"+s.Collection)
	}
	assert.Equal(t, []string{"northwind/Accounts", "northwind/Customers", "chinook/Customer"}, got)
}

func TestDisplayColumns(t *testing.T) {
	key := datatug.PhysicalRef{Source: "northwind", Collection: "Customers", Column: "CustomerID"}

	t.Run("declared skips unmapped, unknown, key and duplicates; caps at six", func(t *testing.T) {
		e := northwindCustomer("ID", "Missing", "City", "CompanyName", "City")
		e.Fields = append(e.Fields, &datatug.EntityField{ID: "Email", Type: "string", Mappings: datatug.PhysicalRefs{{Source: "chinook", Collection: "Customer", Column: "Email"}}})
		e.Display = append(e.Display, "Email")
		got, provenance := DisplayColumns(e, northwindCustomersSchema, key)
		assert.Equal(t, Declared, provenance)
		assert.Equal(t, []DisplayColumn{
			{Column: "City", Field: "City", Label: "City", Provenance: Declared},
			{Column: "CompanyName", Field: "CompanyName", Label: "Company", Provenance: Declared},
		}, got)

		many := &datatug.Entity{}
		var columns []Column
		for _, id := range []string{"a", "b", "c", "d", "e", "f", "g"} {
			many.Fields = append(many.Fields, &datatug.EntityField{ID: id, Type: "string", Mappings: datatug.PhysicalRefs{{Source: "northwind", Collection: "Customers", Column: id}}})
			many.Display = append(many.Display, id)
			columns = append(columns, Column{Name: id, Type: "text"})
		}
		got, _ = DisplayColumns(many, TableSchema{Columns: columns}, key)
		assert.Len(t, got, 6)
	})

	t.Run("declared column absent from schema is skipped", func(t *testing.T) {
		e := northwindCustomer("City")
		schema := TableSchema{PrimaryKey: pk("CustomerID"), Columns: []Column{{Name: "CustomerID", Type: "text"}, {Name: "Phone", Type: "text"}}}
		got, provenance := DisplayColumns(e, schema, key)
		assert.Equal(t, Inferred, provenance)
		assert.Equal(t, []DisplayColumn{{Column: "Phone", Label: "Phone", Provenance: Inferred}}, got)
	})

	t.Run("inferred: mapped fields first, then text columns, at most three", func(t *testing.T) {
		e := northwindCustomer()
		got, provenance := DisplayColumns(e, northwindCustomersSchema, key)
		assert.Equal(t, Inferred, provenance)
		assert.Equal(t, []DisplayColumn{
			{Column: "CompanyName", Field: "CompanyName", Label: "Company", Provenance: Inferred},
			{Column: "City", Field: "City", Label: "City", Provenance: Inferred},
			{Column: "ContactName", Label: "ContactName", Provenance: Inferred},
		}, got)
	})

	t.Run("inferred: entity fields alone fill the cap", func(t *testing.T) {
		e := northwindCustomer()
		for _, id := range []string{"Phone", "Fax"} {
			e.Fields = append(e.Fields, &datatug.EntityField{ID: id, Type: "string", Mappings: datatug.PhysicalRefs{{Source: "northwind", Collection: "Customers", Column: id}}})
		}
		got, provenance := DisplayColumns(e, TableSchema{}, key)
		assert.Equal(t, Inferred, provenance)
		assert.Equal(t, []string{"CompanyName", "City", "Phone"}, []string{got[0].Column, got[1].Column, got[2].Column})
		assert.Len(t, got, 3)
	})

	t.Run("schema text columns only, skipping non-text", func(t *testing.T) {
		e := &datatug.Entity{}
		schema := TableSchema{Columns: []Column{
			{Name: "CustomerID", Type: "text"}, {Name: "Age", Type: "integer"},
			{Name: "A", Type: "text"}, {Name: "B", Type: "text"}, {Name: "C", Type: "text"}, {Name: "D", Type: "text"},
		}}
		got, provenance := DisplayColumns(e, schema, key)
		assert.Equal(t, Inferred, provenance)
		assert.Equal(t, []DisplayColumn{
			{Column: "A", Label: "A", Provenance: Inferred},
			{Column: "B", Label: "B", Provenance: Inferred},
			{Column: "C", Label: "C", Provenance: Inferred},
		}, got)
	})

	t.Run("nothing to show", func(t *testing.T) {
		got, provenance := DisplayColumns(&datatug.Entity{}, TableSchema{Columns: []Column{{Name: "CustomerID", Type: "text"}}}, key)
		assert.Nil(t, got)
		assert.Equal(t, Declared, provenance)
	})
}
