package recordsetcompare

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderRecordset(t *testing.T) {
	columns := []apicontract.Column{{Name: "code", Type: "string"}, {Name: "name", Type: "string"}}
	row := func(code, name string) []apicontract.TypedValue {
		return []apicontract.TypedValue{apicontract.NewStringValue(code), apicontract.NewStringValue(name)}
	}
	rows, err := OrderRecordset(apicontract.Recordset{Columns: columns, Rows: [][]apicontract.TypedValue{row("IE", "Ireland"), row("FR", "France")}}, []string{"code"})
	require.NoError(t, err)
	var codes []string
	for r, err := range rows {
		require.NoError(t, err)
		codes = append(codes, r[0].Str)
	}
	assert.Equal(t, []string{"FR", "IE"}, codes)

	for name, tc := range map[string]struct {
		recordset apicontract.Recordset
		key       []string
	}{
		"no key":        {apicontract.Recordset{Columns: columns}, nil},
		"missing key":   {apicontract.Recordset{Columns: columns}, []string{"nope"}},
		"duplicate col": {apicontract.Recordset{Columns: []apicontract.Column{{Name: "code", Type: "string"}, {Name: "code", Type: "string"}}}, []string{"code"}},
		"short row":     {apicontract.Recordset{Columns: columns, Rows: [][]apicontract.TypedValue{{}}}, []string{"code"}},
		"null key":      {apicontract.Recordset{Columns: columns, Rows: [][]apicontract.TypedValue{{apicontract.NewNullValue(), apicontract.NewStringValue("x")}}}, []string{"code"}},
	} {
		_, err := OrderRecordset(tc.recordset, tc.key)
		assert.Error(t, err, name)
	}
	assert.Error(t, ValidateOrderedRow([]apicontract.TypedValue{apicontract.NewNumberValue(1), apicontract.NewStringValue("x")}, columns, 0))
	assert.NoError(t, ValidateOrderedRow(row("FR", "France"), columns, 0))
}
