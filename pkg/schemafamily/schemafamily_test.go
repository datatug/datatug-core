package schemafamily

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func factory(id string, mutate ...func(*Catalog)) Catalog {
	c := Catalog{ID: id, Collections: []Collection{
		{Name: "countries", Kind: "table", Fields: []Field{{Name: "code", Type: "string", PrimaryKey: true}, {Name: "name", Type: "string"}}},
		{Name: "currencies", Kind: "table", Fields: []Field{{Name: "code", Type: "string", PrimaryKey: true}, {Name: "name", Type: "string"}}},
		{Name: "products", Kind: "table", Fields: []Field{{Name: "id", Type: "int", PrimaryKey: true}, {Name: "sku", Type: "string"}, {Name: "price", Type: "decimal", Nullable: true}}, UniqueKeys: [][]string{{"sku"}}},
		{Name: "orders", Kind: "table", Fields: []Field{{Name: "id", Type: "int", PrimaryKey: true}, {Name: "product_id", Type: "int"}}, ForeignKeys: []ForeignKey{{Columns: []string{"product_id"}, Collection: "products", RefColumns: []string{"id"}}}},
	}}
	for _, m := range mutate {
		m(&c)
	}
	return c
}

func TestFingerprintIgnoresOrderAndNames(t *testing.T) {
	a := factory("a")
	b := factory("b", func(c *Catalog) {
		c.Collections[0], c.Collections[3] = c.Collections[3], c.Collections[0]
		c.Collections[2].Fields[0], c.Collections[2].Fields[2] = c.Collections[2].Fields[2], c.Collections[2].Fields[0]
	})
	fa, fb := Compute(a), Compute(b)
	assert.Equal(t, fa, fb)
	assert.Regexp(t, `^fp1:types:[0-9a-f]{64}$`, fa.Types)
	require.NotNil(t, fa.Keys)
	assert.Regexp(t, `^fp1:keys:[0-9a-f]{64}$`, *fa.Keys)

	// A key change alters only the keys level.
	c := factory("c", func(c *Catalog) { c.Collections[2].UniqueKeys = nil })
	fc := Compute(c)
	assert.Equal(t, fa.Types, fc.Types)
	assert.NotEqual(t, *fa.Keys, *fc.Keys)

	d := factory("d", func(c *Catalog) { c.KeysUnavailable = "foreign keys not read" })
	fd := Compute(d)
	assert.Nil(t, fd.Keys)
	assert.Equal(t, "foreign keys not read", fd.KeysUnavailable)
	assert.Equal(t, "app.t", Collection{Schema: "app", Name: "t"}.QualifiedName())
}

func TestClassifyBands(t *testing.T) {
	families := []Family{{ID: "factory", Catalog: factory("ref")}, {ID: "chinook", Catalog: Catalog{ID: "chinook", Collections: []Collection{
		{Name: "Artist", Fields: []Field{{Name: "ArtistId", Type: "int", PrimaryKey: true}}},
		{Name: "Album", Fields: []Field{{Name: "AlbumId", Type: "int", PrimaryKey: true}}},
		{Name: "Track", Fields: []Field{{Name: "TrackId", Type: "int", PrimaryKey: true}}},
	}}}}

	m := Classify(factory("new-site"), families, Default)
	require.Len(t, m, 1)
	assert.Equal(t, BandIdentical, m[0].Band)
	assert.Equal(t, "factory", m[0].Family)
	assert.Equal(t, "likely", m[0].Confidence)
	assert.Equal(t, "keys", m[0].Evidence.FingerprintLevel)

	noKeys := factory("x", func(c *Catalog) { c.KeysUnavailable = "not read" })
	m = Classify(noKeys, families, Default)
	assert.Equal(t, BandIdentical, m[0].Band)
	assert.Equal(t, "types", m[0].Evidence.FingerprintLevel)
	assert.Equal(t, "possible", m[0].Confidence)

	// v2 adds a column and a table: same system, different version.
	v2 := factory("v2", func(c *Catalog) {
		c.Collections[2].Fields = append(c.Collections[2].Fields, Field{Name: "barcode", Type: "string"})
		c.Collections = append(c.Collections, Collection{Name: "units", Kind: "table", Fields: []Field{{Name: "code", Type: "string", PrimaryKey: true}}})
	})
	m = Classify(v2, families, Default)
	assert.Equal(t, BandSameSystemDifferentVersion, m[0].Band)
	assert.Equal(t, "likely", m[0].Confidence)
	assert.Equal(t, []string{"units"}, m[0].Evidence.OnlyHere)
	assert.Equal(t, 80, m[0].Evidence.CollectionOverlapPc)

	// A changed primary key lowers confidence.
	pk := factory("pk", func(c *Catalog) {
		c.Collections[1].Fields[1].PrimaryKey = true
		c.Collections[2].Fields = append(c.Collections[2].Fields, Field{Name: "barcode", Type: "string"})
	})
	m = Classify(pk, families, Default)
	assert.Equal(t, BandSameSystemDifferentVersion, m[0].Band)
	assert.Equal(t, "possible", m[0].Confidence)

	// Mostly different tables but shared identifiers: related, distinct.
	related := Catalog{ID: "erp", Collections: []Collection{
		{Name: "products", Fields: []Field{{Name: "id", Type: "int"}, {Name: "sku", Type: "string"}}},
		{Name: "countries", Fields: []Field{{Name: "code", Type: "string"}}},
		{Name: "invoices", Fields: []Field{{Name: "id", Type: "int"}}},
		{Name: "ledger", Fields: []Field{{Name: "id", Type: "int"}}},
		{Name: "accounts", Fields: []Field{{Name: "id", Type: "int"}}},
		{Name: "orders", Fields: []Field{{Name: "id", Type: "string"}, {Name: "product_id", Type: "int"}}},
	}}
	m = Classify(related, families, Default)
	assert.Equal(t, BandRelatedDistinct, m[0].Band)
	assert.Contains(t, m[0].Evidence.TypeChanges, "orders.id: int → string")

	m = Classify(Catalog{ID: "z", Collections: []Collection{{Name: "a"}, {Name: "b"}, {Name: "c"}}}, families, Default)
	assert.Equal(t, []Match{{Band: BandNew}}, m)

	m = Classify(Catalog{ID: "tiny", Collections: []Collection{{Name: "a"}}}, families, Default)
	assert.Equal(t, BandInsufficientEvidence, m[0].Band)
}

func TestClassifyOrdersBestFirst(t *testing.T) {
	v2 := factory("v2", func(c *Catalog) {
		c.Collections[2].Fields = append(c.Collections[2].Fields, Field{Name: "barcode", Type: "string"})
	})
	families := []Family{{ID: "zeta", Catalog: v2}, {ID: "alpha", Catalog: v2}, {ID: "factory", Catalog: factory("ref")}}
	m := Classify(factory("x"), families, Default)
	require.Len(t, m, 3)
	assert.Equal(t, []string{"factory", "alpha", "zeta"}, []string{m[0].Family, m[1].Family, m[2].Family})

	v3 := factory("v3", func(c *Catalog) {
		c.Collections[2].Fields = append(c.Collections[2].Fields, Field{Name: "barcode", Type: "string"}, Field{Name: "color", Type: "string"})
	})
	m = Classify(factory("y"), []Family{{ID: "v3", Catalog: v3}, {ID: "v2", Catalog: v2}}, Default)
	assert.Equal(t, "v2", m[0].Family, "higher field overlap first")
	assert.Equal(t, 0.0, ratio(1, 0))
}

func TestIsIdentifier(t *testing.T) {
	for _, name := range []string{"id", "code", "sku", "product_id", "ArtistId", "country_code"} {
		assert.True(t, isIdentifier(name), name)
	}
	for _, name := range []string{"name", "paid", "Id2", "valid"} {
		assert.False(t, isIdentifier(name), name)
	}
}

func TestReconcileSchemas(t *testing.T) {
	result := ReconcileSchemas([]Catalog{
		factory("a"), factory("b"), factory("c"),
		factory("e", func(c *Catalog) {
			c.Collections[2].Fields = append(c.Collections[2].Fields, Field{Name: "barcode", Type: "string"})
			c.Collections[1].Fields[1].Type = "text"
			c.Collections = c.Collections[1:] // no countries
		}),
	})
	assert.Equal(t, []string{"a", "b", "c", "e"}, result.Instances)
	require.Len(t, result.Groups, 2)
	assert.Equal(t, []string{"a", "b", "c"}, result.Groups[0].Instances)
	assert.Equal(t, []SchemaDrift{
		{Collection: "countries", MissingIn: []string{"e"}},
		{Collection: "currencies", Field: "name", Types: map[string]string{"a": "string", "b": "string", "c": "string", "e": "text"}},
		{Collection: "products", Field: "barcode", MissingIn: []string{"a", "b", "c"}},
	}, result.Drift)

	same := ReconcileSchemas([]Catalog{factory("a", func(c *Catalog) { c.KeysUnavailable = "x" }), factory("b")})
	require.Len(t, same.Groups, 1, "types level is used when any instance lacks keys")
	assert.Empty(t, same.Drift)

	split := ReconcileSchemas([]Catalog{factory("a"), factory("b", func(c *Catalog) { c.Collections = c.Collections[1:] })})
	require.Len(t, split.Groups, 2)
	assert.Less(t, split.Groups[0].Fingerprint, split.Groups[1].Fingerprint, "equal-size groups order by fingerprint")
}
