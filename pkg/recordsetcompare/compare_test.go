package recordsetcompare

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/require"
)

func receipt(id string, rows int) apicontract.CompareSideReceipt {
	return apicontract.CompareSideReceipt{Execution: apicontract.ExecutionRef{StoreID: "evidence", ProjectID: "orders", ExecutionID: id}, ExecutedAt: "2026-09-14T10:00:00Z", RowCount: rows, Reproducible: true}
}

func recordset(columns []apicontract.Column, rows ...[]apicontract.TypedValue) apicontract.Recordset {
	return apicontract.Recordset{Columns: columns, Rows: rows}
}

func TestCompareTypedValuesCompositeKeysAndStableOrdering(t *testing.T) {
	columns := []apicontract.Column{{Name: "tenant", Type: "string"}, {Name: "id", Type: "integer"}, {Name: "value", Type: "string"}}
	left := recordset(columns,
		[]apicontract.TypedValue{apicontract.NewStringValue("b"), apicontract.NewIntegerValue("2"), apicontract.NewStringValue("removed")},
		[]apicontract.TypedValue{apicontract.NewStringValue("a"), apicontract.NewIntegerValue("1"), apicontract.NewStringValue("old")},
		[]apicontract.TypedValue{apicontract.NewStringValue("a"), apicontract.NewIntegerValue("3"), apicontract.NewStringValue("same")},
	)
	right := recordset(columns,
		[]apicontract.TypedValue{apicontract.NewStringValue("c"), apicontract.NewIntegerValue("4"), apicontract.NewStringValue("added")},
		[]apicontract.TypedValue{apicontract.NewStringValue("a"), apicontract.NewIntegerValue("3"), apicontract.NewStringValue("same")},
		[]apicontract.TypedValue{apicontract.NewStringValue("a"), apicontract.NewIntegerValue("1"), apicontract.NewStringValue("new")},
	)
	result, err := Compare(left, right, receipt("left", 3), receipt("right", 3), Options{Key: []string{"tenant", "id"}})
	require.NoError(t, err)
	require.Equal(t, apicontract.CompareSummary{Added: 1, Removed: 1, Changed: 1, Unchanged: 1, ColumnsOnlyOnOneSide: []apicontract.CompareOneSidedColumn{}}, result.Summary)
	require.Equal(t, []apicontract.TypedValue{apicontract.NewStringValue("c"), apicontract.NewIntegerValue("4")}, result.Added[0].Key)
	require.Equal(t, []apicontract.TypedValue{apicontract.NewStringValue("b"), apicontract.NewIntegerValue("2")}, result.Removed[0].Key)
	require.Equal(t, []apicontract.TypedValue{apicontract.NewStringValue("a"), apicontract.NewIntegerValue("1")}, result.Changed[0].Key)
	require.Equal(t, "value", result.Changed[0].Columns[0].Column)
}

func TestCompareAllTypedValueKindsUseExactTypedEquality(t *testing.T) {
	values := []apicontract.TypedValue{
		apicontract.NewStringValue("x"), apicontract.NewNumberValue(1.25), apicontract.NewIntegerValue("7"),
		apicontract.NewDecimalValue("1.00"), apicontract.NewBooleanValue(false), apicontract.NewDateValue("2026-09-14"),
		apicontract.NewDatetimeValue("2026-09-14T10:00:00Z"), apicontract.NewNullValue(),
	}
	columns := []apicontract.Column{{Name: "id", Type: "string"}}
	for index, value := range values {
		columns = append(columns, apicontract.Column{Name: string(rune('a' + index)), Type: declaredType(value)})
	}
	row := []apicontract.TypedValue{apicontract.NewStringValue("row")}
	row = append(row, values...)
	result, err := Compare(recordset(columns, row), recordset(columns, append([]apicontract.TypedValue(nil), row...)), receipt("left", 1), receipt("right", 1), Options{Key: []string{"id"}})
	require.NoError(t, err)
	require.Equal(t, 1, result.Summary.Unchanged)

	changed := append([]apicontract.TypedValue(nil), row...)
	changed[4] = apicontract.NewDecimalValue("1.0")
	result, err = Compare(recordset(columns, row), recordset(columns, changed), receipt("left", 1), receipt("right", 1), Options{Key: []string{"id"}})
	require.NoError(t, err)
	require.Equal(t, 1, result.Summary.Changed, "canonical decimals compare exactly, not numerically")
	require.Equal(t, "d", result.Changed[0].Columns[0].Column)
}

func declaredType(value apicontract.TypedValue) string {
	if value.Type == apicontract.ValueTypeNull {
		return "string"
	}
	return string(value.Type)
}

func TestCompareSharedColumnsOneSidedSummaryAndAlignedRows(t *testing.T) {
	leftColumns := []apicontract.Column{{Name: "left_only", Type: "string"}, {Name: "id", Type: "integer"}, {Name: "name", Type: "string"}}
	rightColumns := []apicontract.Column{{Name: "name", Type: "string"}, {Name: "right_only", Type: "boolean"}, {Name: "id", Type: "integer"}}
	left := recordset(leftColumns, []apicontract.TypedValue{apicontract.NewStringValue("hidden"), apicontract.NewIntegerValue("1"), apicontract.NewStringValue("removed")})
	right := recordset(rightColumns, []apicontract.TypedValue{apicontract.NewStringValue("added"), apicontract.NewBooleanValue(true), apicontract.NewIntegerValue("2")})
	result, err := Compare(left, right, receipt("left", 1), receipt("right", 1), Options{Key: []string{"id"}})
	require.NoError(t, err)
	require.Equal(t, []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "name", Type: "string"}}, result.Columns)
	require.Equal(t, []apicontract.TypedValue{apicontract.NewIntegerValue("2"), apicontract.NewStringValue("added")}, result.Added[0].Row)
	require.Equal(t, []apicontract.CompareOneSidedColumn{{Column: "left_only", Side: "left"}, {Column: "right_only", Side: "right"}}, result.Summary.ColumnsOnlyOnOneSide)
}

func TestCompareRejectsDuplicateMissingAndNullKeys(t *testing.T) {
	columns := []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "name", Type: "string"}}
	valid := recordset(columns, []apicontract.TypedValue{apicontract.NewIntegerValue("1"), apicontract.NewStringValue("x")})
	duplicate := recordset(columns, valid.Rows[0], valid.Rows[0])
	_, err := Compare(duplicate, valid, receipt("left", 2), receipt("right", 1), Options{Key: []string{"id"}})
	require.ErrorContains(t, err, "duplicate key")
	_, err = Compare(valid, valid, receipt("left", 1), receipt("right", 1), Options{Key: []string{"missing"}})
	require.ErrorContains(t, err, "missing from the shared columns")
	nullKey := recordset(columns, []apicontract.TypedValue{apicontract.NewNullValue(), apicontract.NewStringValue("x")})
	_, err = Compare(nullKey, valid, receipt("left", 1), receipt("right", 1), Options{Key: []string{"id"}})
	require.ErrorContains(t, err, "null/missing key")
}

func TestCompareLimitAndDistributionAreDeterministic(t *testing.T) {
	columns := []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "state", Type: "string"}}
	left := recordset(columns,
		[]apicontract.TypedValue{apicontract.NewIntegerValue("1"), apicontract.NewStringValue("open")},
		[]apicontract.TypedValue{apicontract.NewIntegerValue("2"), apicontract.NewStringValue("open")},
	)
	right := recordset(columns,
		[]apicontract.TypedValue{apicontract.NewIntegerValue("1"), apicontract.NewStringValue("closed")},
		[]apicontract.TypedValue{apicontract.NewIntegerValue("3"), apicontract.NewStringValue("closed")},
	)
	result, err := Compare(left, right, receipt("left", 2), receipt("right", 2), Options{Key: []string{"id"}, Limit: 1, DistributionColumn: "state"})
	require.NoError(t, err)
	require.True(t, result.Truncated)
	require.Equal(t, 3, result.Summary.Added+result.Summary.Removed+result.Summary.Changed)
	require.Len(t, result.Added, 0)
	require.Len(t, result.Removed, 0)
	require.Len(t, result.Changed, 1)
	require.Equal(t, "closed", result.Distribution.Values[0].Value.Str)
	require.Equal(t, 2, result.Distribution.Values[0].Right.Count)
	require.Equal(t, float64(100), result.Distribution.Values[0].Right.Pct)
	require.NotNil(t, result.Distribution.Values[0].Ratio)
	require.Equal(t, float64(0), *result.Distribution.Values[0].Ratio)
	require.Equal(t, "open", result.Distribution.Values[1].Value.Str)
	require.Equal(t, float64(0), result.Distribution.Values[1].Right.Pct)
	require.Nil(t, result.Distribution.Values[1].Ratio)

	repeated, err := Compare(left, right, receipt("left", 2), receipt("right", 2), Options{Key: []string{"id"}, Limit: 1, DistributionColumn: "state"})
	require.NoError(t, err)
	require.Equal(t, result, repeated)
}

func TestCompareDistributionCapsAndOrdersValues(t *testing.T) {
	columns := []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "bucket", Type: "string"}}
	rows := make([][]apicontract.TypedValue, 0, apicontract.CompareDistributionMaximumValues+2)
	for i := apicontract.CompareDistributionMaximumValues + 1; i >= 0; i-- {
		rows = append(rows, []apicontract.TypedValue{apicontract.NewIntegerValue(strconv.Itoa(i)), apicontract.NewStringValue(string(rune('A' + i)))})
	}
	set := recordset(columns, rows...)
	result, err := Compare(set, set, receipt("left", len(rows)), receipt("right", len(rows)), Options{Key: []string{"id"}, DistributionColumn: "bucket"})
	require.NoError(t, err)
	require.True(t, result.Distribution.Truncated)
	require.Len(t, result.Distribution.Values, apicontract.CompareDistributionMaximumValues)
	for i := 1; i < len(result.Distribution.Values); i++ {
		require.Less(t, result.Distribution.Values[i-1].Value.Str, result.Distribution.Values[i].Value.Str)
	}
}

func TestComparePolicyLimitedOnlyWhenRowsFiltered(t *testing.T) {
	columns := []apicontract.Column{{Name: "id", Type: "integer"}}
	set := recordset(columns, []apicontract.TypedValue{apicontract.NewIntegerValue("1")})
	left := receipt("left", 1)
	left.Limitations = []apicontract.Limitation{{Policy: "columns", HiddenColumns: []string{"secret"}}}
	result, err := Compare(set, set, left, receipt("right", 1), Options{Key: []string{"id"}})
	require.NoError(t, err)
	require.False(t, result.PolicyLimited)
	left.Limitations[0].RowsFiltered = true
	result, err = Compare(set, set, left, receipt("right", 1), Options{Key: []string{"id"}})
	require.NoError(t, err)
	require.True(t, result.PolicyLimited)
}

func TestCompareHiddenColumnAppearsNowhere(t *testing.T) {
	leftColumns := []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "name", Type: "string"}}
	rightColumns := append(append([]apicontract.Column{}, leftColumns...), apicontract.Column{Name: "email", Type: "string"})
	left := recordset(leftColumns, []apicontract.TypedValue{apicontract.NewIntegerValue("1"), apicontract.NewStringValue("Alice")})
	right := recordset(rightColumns, []apicontract.TypedValue{apicontract.NewIntegerValue("1"), apicontract.NewStringValue("Alice"), apicontract.NewStringValue("secret@example.com")})
	leftReceipt := receipt("left", 1)
	leftReceipt.Limitations = []apicontract.Limitation{{Policy: "masked-fields", HiddenColumns: []string{"email"}}}

	result, err := Compare(left, right, leftReceipt, receipt("right", 1), Options{Key: []string{"id"}})
	require.NoError(t, err)
	require.Equal(t, leftColumns, result.Columns)
	require.Empty(t, result.Summary.ColumnsOnlyOnOneSide)
	wire, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(wire), "email")
	require.NotContains(t, string(wire), "secret@example.com")

	for _, options := range []Options{{Key: []string{"email"}}, {Key: []string{"id"}, DistributionColumn: "email"}} {
		_, err = Compare(left, right, leftReceipt, receipt("right", 1), options)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "email")
	}
}

func TestCompareOmittedLimitEmitsOnlyDefaultHundred(t *testing.T) {
	columns := []apicontract.Column{{Name: "id", Type: "integer"}}
	rows := make([][]apicontract.TypedValue, 0, 101)
	for i := 0; i < 101; i++ {
		rows = append(rows, []apicontract.TypedValue{apicontract.NewIntegerValue(strconv.Itoa(i))})
	}
	result, err := Compare(recordset(columns), recordset(columns, rows...), receipt("left", 0), receipt("right", 101), Options{Key: []string{"id"}})
	require.NoError(t, err)
	require.Equal(t, 100, apicontract.CompareDefaultLimit)
	require.Len(t, result.Added, 100)
	require.Equal(t, 101, result.Summary.Added)
	require.True(t, result.Truncated)
}
