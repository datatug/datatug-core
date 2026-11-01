package dataaccess

import (
	"math"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/require"
)

func TestStructuredOperationParametersRemainExactAndBound(t *testing.T) {
	for _, test := range []struct {
		wire string
		want any
	}{
		{`{"type":"string","value":"Ada"}`, "Ada"},
		{`{"type":"integer","value":"9007199254740991"}`, int64(9007199254740991)},
		{`{"type":"number","value":1.25}`, 1.25},
		{`{"type":"decimal","value":"0.5"}`, 0.5},
		{`{"type":"boolean","value":true}`, true},
		{`{"type":"date","value":"2026-11-01"}`, "2026-11-01"},
		{`{"type":"datetime","value":"2026-11-01T12:00:00Z"}`, "2026-11-01T12:00:00Z"},
		{`{"type":"null","value":null}`, nil},
		{`{"type":"set","values":[{"type":"integer","value":"1"},{"type":"integer","value":"2"}]}`, []any{int64(1), int64(2)}},
	} {
		values, err := structuredParameters(`{"p":` + test.wire + `}`)
		require.NoError(t, err)
		require.Equal(t, test.want, values["p"])
		op := structuredOperation()
		op.Parameters = `{"p":` + test.wire + `}`
		op.TugQL = "from: {schema: sales, name: Customer}\nwhere: {op: '==', left: {field: id}, right: {param: p}}\ncolumns: [{field: id}]"
		q, _, err := op.query()
		require.NoError(t, err)
		require.NotNil(t, q)
	}
	for _, wire := range []string{
		`null`, `[]`, `{"p":{"type":"integer","value":"9007199254740992"}}`,
		`{"p":{"type":"integer","value":"9999999999999999999999999999999"}}`,
		`{"p":{"type":"number","value":9007199254740992}}`,
		`{"p":{"type":"decimal","value":"0.1"}}`,
		`{"p":{"type":"decimal","value":"999999999999999999999999999999999999999999"}}`,
		`{"p":{"type":"set","values":[{"type":"decimal","value":"0.1"}]}}`,
		`{"p":{"type":"string","value":"x"},"p":{"type":"string","value":"y"}}`,
		`{"p":{"type":"number","value":1,"extra":0}}`,
		`{"principal.roles":{"type":"string","value":"owner"}}`,
		`{"not-valid!":{"type":"string","value":"x"}}`,
	} {
		op := structuredOperation()
		op.Parameters = wire
		_, _, err := op.query()
		require.Error(t, err, wire)
	}
	_, err := structuredScalar(apicontract.TypedValue{})
	require.Error(t, err)
	_, err = structuredScalar(apicontract.NewNumberValue(math.Inf(1)))
	require.Error(t, err)
	for _, op := range []StructuredRead{{}, {TugQL: strings.Repeat("x", 131073), MaxRows: 1, MaxBytes: 1}, {TugQL: "x", MaxRows: 101, MaxBytes: 1}, {TugQL: "x", MaxRows: 1, MaxBytes: 262145}, {TugQL: "x", Parameters: strings.Repeat("x", 65537), MaxRows: 1, MaxBytes: 1}} {
		_, _, err := op.query()
		require.Error(t, err)
		_, err = op.RequestFor("valid-fresh-nonce")
		require.Error(t, err)
	}
	_, err = structuredOperation().RequestFor("short")
	require.Error(t, err)
	op := structuredOperation()
	op.TugQL = "from: {schema: sales, name: Customer}\nwhere: {op: '==', left: {field: id}, right: {param: missing}}"
	op.Parameters = `{"other":{"type":"integer","value":"1"}}`
	_, _, err = op.query()
	require.Error(t, err)
	// Unrelated bindings must not refuse an otherwise supported literal/null filter.
	op.TugQL = "from: {schema: sales, name: Customer}\nwhere: {isNull: {field: id}}"
	_, _, err = op.query()
	require.NoError(t, err)
}

type unencodableStructuredQuery struct{ dal.StructuredQuery }

func (q unencodableStructuredQuery) Columns() []dal.Column {
	return []dal.Column{{Expression: dal.Constant{Value: make(chan int)}, Alias: "x"}}
}

func TestStructuredFreezeRejectsUnknownAndUnresolvedShapes(t *testing.T) {
	op := structuredOperation()
	q, _, err := op.query()
	require.NoError(t, err)
	_, _, err = freezeStructuredOperation(unencodableStructuredQuery{q})
	require.Error(t, err)
	_, _, err = parseStructuredOperation([]byte("bad: query"))
	require.Error(t, err)
	_, _, err = parseStructuredOperation([]byte("from: {schema: sales, name: Customer}\ncolumns: [{param: x, as: x}]"))
	require.Error(t, err)
	op.TugQL = "from: {query: {as: nested, from: {schema: sales, name: Customer}, columns: [{field: id}]} }\ncolumns: [{field: id, source: nested}]"
	_, _, err = op.query()
	require.ErrorContains(t, err, "separately owned")
	_, err = structuredIdentities(nil)
	require.Error(t, err)
	_, err = structuredIdentities([]access.Resource{access.OpaqueQuery("unknown")})
	require.Error(t, err)
	_, err = structuredIdentities([]access.Resource{access.CollectionResourceFor(nil, "")})
	require.Error(t, err)
}
