package datatug

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

func TestNamedEntityTablesLosslessWireAndLegacyName(t *testing.T) {
	var legacy TableKeys
	require.NoError(t, json.Unmarshal([]byte(`[{"name":"Order Details"}]`), &legacy))
	require.Equal(t, CollectionTypeUnknown, legacy[0].Type())
	body, err := json.Marshal(legacy)
	require.NoError(t, err)
	require.JSONEq(t, `[{"name":"Order Details"}]`, string(body))
	parent := record.NewKeyWithParentAndID(record.NewKeyWithID("databases", "shipping"), "tenants", int64(9007199254740993))
	keys := TableKeys{NewTableKey("routes", "logical schema", "catalog", parent), {t: CollectionTypeView, schema: "logical", catalog: "catalog", Ref: dal.NewDatabaseCollectionRef("database", "physical", "requests", "")}}
	body, err = json.Marshal(keys)
	require.NoError(t, err)
	require.Contains(t, string(body), `"id":"9007199254740993","idType":"int64"`)
	var got TableKeys
	require.NoError(t, json.Unmarshal(body, &got))
	for i := range keys {
		require.Equal(t, keys[i].Type(), got[i].Type())
		require.Equal(t, keys[i].Name(), got[i].Name())
		require.Equal(t, keys[i].Schema(), got[i].Schema())
		require.Equal(t, keys[i].Catalog(), got[i].Catalog())
		require.Equal(t, keys[i].Ref.Database(), got[i].Ref.Database())
		require.Equal(t, keys[i].Ref.Schema(), got[i].Ref.Schema())
	}
	require.Equal(t, parent.String(), got[0].Ref.Parent().String())
	require.IsType(t, int64(0), got[0].Ref.Parent().ID)
	for _, number := range []any{int(-1), int8(-2), int16(-3), int32(-4), int64(-5), uint(1), uint8(2), uint16(3), uint32(4), uint64(9007199254740993)} {
		t.Run(fmt.Sprintf("%T", number), func(t *testing.T) {
			p := record.NewKeyWithParentAndID(nil, "ids", number)
			tables := TableKeys{NewTableKey("data", "", "", p)}
			wire, err := json.Marshal(tables)
			require.NoError(t, err)
			var decoded TableKeys
			require.NoError(t, json.Unmarshal(wire, &decoded))
			require.Equal(t, number, decoded[0].Ref.Parent().ID)
		})
	}
	for _, wire := range []string{`null`, `[]`, `[{"name":"x","Ref":{}}]`} {
		require.NoError(t, json.Unmarshal([]byte(wire), &got))
	}
	var nilTables TableKeys
	body, err = json.Marshal(nilTables)
	require.NoError(t, err)
	require.Equal(t, "null", string(body))
}
func TestNamedTableCodecCannotHideEmbeddedOwnerSiblings(t *testing.T) {
	values := []any{TableModel{DBCollectionKey: NewTableKey("t", "", "", nil), DbType: "BASE TABLE", Columns: ColumnModels{}}, CollectionInfo{DDL: "CREATE TABLE t", TableProps: TableProps{DbType: "BASE TABLE"}}, ReferencedBy{ForeignKeys: []*RefByForeignKey{}}}
	for i, value := range values {
		body, err := json.Marshal(value)
		require.NoError(t, err)
		switch i {
		case 0:
			require.Contains(t, string(body), `"dbType":"BASE TABLE"`)
			require.Contains(t, string(body), `"Columns"`)
		case 1:
			require.Contains(t, string(body), `"ddl":"CREATE TABLE t"`)
		case 2:
			require.Contains(t, string(body), `"foreignKeys":[]`)
		}
	}
}
func TestNamedEntityTablesRejectMalformedAndRuntimeWire(t *testing.T) {
	for _, body := range []string{`{}`, `null trailing`, `[`, `[{"name":"x","name":"y"}]`, `[{"name":"x","unknown":1}]`, `[{"name":"x","Ref":{"password":"secret"}}]`, `[{"Ref":{}}]`, `[{"name":"x","type":"invalid"}]`, `[{"name":"x","parent":[{"collection":"","id":"x"}]}]`, `[{"name":"x","parent":[{"collection":"c","id":"01","idType":"int64"}]}]`, `[{"name":"x","parent":[{"collection":"c","id":"1","idType":"float64"}]}]`, `[{"name":"x","parent":[{"collection":"c","id":"-1","idType":"uint"}]}]`, `[{"name":"x","database":"d","parent":[{"collection":"c","id":"1"}]}]`} {
		t.Run(body, func(t *testing.T) { var value TableKeys; require.Error(t, json.Unmarshal([]byte(body), &value)) })
	}
	for _, ref := range []dal.CollectionRef{dal.NewCollectionRef("x", "alias", nil), dal.NewCollectionRef("x", "", nil).WithScan(1), dal.NewCollectionRef("x", "", record.NewKeyWithID("c", true))} {
		_, err := json.Marshal(TableKeys{{Ref: ref}})
		require.Error(t, err)
	}
	_, err := json.Marshal(TableKeys{{}})
	require.Error(t, err)
	var value TableKeys
	require.Error(t, json.Unmarshal([]byte(`[{"name":"`+strings.Repeat("x", 256<<10)+`"}]`), &value))
}

func TestEntityTableCodecBoundsAndSyntaxFailures(t *testing.T) {
	_, err := json.Marshal(TableKeys{NewTableKey(strings.Repeat("x", 256<<10), "", "", nil)})
	require.Error(t, err)
	var parent *record.Key
	for range 17 {
		parent = record.NewKeyWithParentAndID(parent, "p", "id")
	}
	_, err = json.Marshal(TableKeys{NewTableKey("table", "", "", parent)})
	require.Error(t, err)
	_, err = json.Marshal(TableKeys{NewTableKey("table", "", "", record.NewKeyWithID("p", ""))})
	require.Error(t, err)
	var keys TableKeys
	require.NoError(t, json.Unmarshal([]byte(`[{"name":"t","schema":"logical","refSchema":"physical"}]`), &keys))
	require.Equal(t, "physical", keys[0].Ref.Schema())
	require.Equal(t, "logical", keys[0].Schema())
	for _, wire := range []string{`}`, `{"x`, `[{"name":`, `[{"name":"x",`, `[{"name":"x","parent":[`, `null null`, strings.Repeat("[", 42) + strings.Repeat("]", 42)} {
		require.Error(t, validateEntityTableJSON([]byte(wire)), "wire %q", wire)
	}
}
func TestAssetNumberParameterDefaultsRemainExact(t *testing.T) {
	for _, test := range []struct {
		kind, value string
		valid       bool
	}{{"integer", "9007199254740993", true}, {"integer", "9223372036854775808", false}, {"integer", "1.2", false}, {"number", "0.12345678901234567890123456789", true}, {"number", "1e9999", false}, {"number", "invalid", false}} {
		err := (ParameterDef{ID: "p", Type: test.kind, DefaultValue: json.Number(test.value)}).Validate()
		require.Equal(t, test.valid, err == nil)
	}
	require.Error(t, (ParameterDef{ID: "p", Type: "number", DefaultValue: "not numeric"}).Validate())
}
