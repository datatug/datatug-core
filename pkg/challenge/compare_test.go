package challenge

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/require"
)

func columnsOf(types ...string) []apicontract.Column {
	columns := make([]apicontract.Column, len(types))
	for index, typ := range types {
		columns[index] = apicontract.Column{Name: string(rune('a' + index)), Type: typ}
	}
	return columns
}

func row(values ...apicontract.TypedValue) []apicontract.TypedValue {
	return values
}

func integer(value string) apicontract.TypedValue { return apicontract.NewIntegerValue(value) }
func number(value float64) apicontract.TypedValue { return apicontract.NewNumberValue(value) }
func text(value string) apicontract.TypedValue    { return apicontract.NewStringValue(value) }
func null() apicontract.TypedValue                { return apicontract.NewNullValue() }

func TestCompareTypedRowsIsAMultisetByDefault(t *testing.T) {
	columns := columnsOf("integer", "string")
	expected := [][]apicontract.TypedValue{
		row(integer("1"), text("a")),
		row(integer("2"), text("b")),
		row(integer("2"), text("b")),
	}
	actual := [][]apicontract.TypedValue{
		row(integer("2"), text("b")),
		row(integer("1"), text("a")),
		row(integer("2"), text("b")),
	}
	diff := CompareTypedRows(columns, columns, expected, actual, RowOptions{Ordering: OrderUnordered})
	require.True(t, diff.Match, diff.Message)

	missingMultiplicity := actual[:2]
	diff = CompareTypedRows(columns, columns, expected, missingMultiplicity, RowOptions{Ordering: OrderUnordered})
	require.False(t, diff.Match)
	require.Equal(t, ProblemRowContent, diff.Problem)
	require.Equal(t, 1, diff.Missing)
	require.Zero(t, diff.Unexpected)
}

func TestCompareTypedRowsAssertsOrderOnlyWhenDeclared(t *testing.T) {
	columns := columnsOf("integer")
	expected := [][]apicontract.TypedValue{row(integer("1")), row(integer("2"))}
	reversed := [][]apicontract.TypedValue{row(integer("2")), row(integer("1"))}

	require.True(t, CompareTypedRows(columns, columns, expected, reversed, RowOptions{Ordering: OrderUnordered}).Match)
	diff := CompareTypedRows(columns, columns, expected, reversed, RowOptions{Ordering: OrderOrdered})
	require.False(t, diff.Match)
	require.Equal(t, ProblemRowOrder, diff.Problem)
}

func TestCompareTypedRowsKeepsNullAndStorageClassDistinct(t *testing.T) {
	columns := columnsOf("string")
	expected := [][]apicontract.TypedValue{row(null()), row(text(""))}
	actual := [][]apicontract.TypedValue{row(null()), row(text(""))}
	require.True(t, CompareTypedRows(columns, columns, expected, actual, RowOptions{Ordering: OrderUnordered}).Match)

	emptyForNull := [][]apicontract.TypedValue{row(text("")), row(text(""))}
	require.False(t, CompareTypedRows(columns, columns, expected, emptyForNull, RowOptions{Ordering: OrderUnordered}).Match)

	expectedInteger := columnsOf("integer")
	actualReal := columnsOf("number")
	expectedRows := [][]apicontract.TypedValue{row(integer("2"))}
	actualRows := [][]apicontract.TypedValue{row(number(2))}

	diff := CompareTypedRows(expectedInteger, actualReal, expectedRows, actualRows, RowOptions{Ordering: OrderUnordered})
	require.False(t, diff.Match)
	require.Equal(t, ProblemColumnType, diff.Problem)

	tolerated := CompareTypedRows(expectedInteger, actualReal, expectedRows, actualRows, RowOptions{Ordering: OrderUnordered, NumericTolerance: "0.01"})
	require.True(t, tolerated.Match, tolerated.Message)

	// An empty result cannot expose a storage class, so declared observed
	// column types are not asserted without values.
	emptyExpected := [][]apicontract.TypedValue{}
	emptyActual := [][]apicontract.TypedValue{}
	require.True(t, CompareTypedRows(expectedInteger, actualReal, emptyExpected, emptyActual, RowOptions{Ordering: OrderUnordered}).Match)
	namedExpected := []apicontract.Column{{Name: "expected_name", Type: "integer"}}
	namedActual := []apicontract.Column{{Name: "observed_alias", Type: "number"}}
	require.False(t, CompareTypedRows(namedExpected, namedActual, emptyExpected, emptyActual, RowOptions{Ordering: OrderUnordered, RequireColumnNames: true}).Match)

	// Canonical decimal 2.0 differs from integer 2 unless tolerance is declared.
	expectedDecimal := columnsOf("decimal")
	actualInteger := columnsOf("integer")
	expectedDecimalRows := [][]apicontract.TypedValue{row(apicontract.NewDecimalValue("2.0"))}
	actualIntegerRows := [][]apicontract.TypedValue{row(integer("2"))}
	require.False(t, CompareTypedRows(expectedDecimal, actualInteger, expectedDecimalRows, actualIntegerRows, RowOptions{Ordering: OrderUnordered}).Match)
	require.True(t, CompareTypedRows(expectedDecimal, actualInteger, expectedDecimalRows, actualIntegerRows, RowOptions{Ordering: OrderUnordered, NumericTolerance: "0.01"}).Match)
}

func TestCompareTypedRowsFindsAMaximumMatchUnderTolerance(t *testing.T) {
	columns := columnsOf("number")
	expected := [][]apicontract.TypedValue{row(number(1.0)), row(number(1.8))}
	actual := [][]apicontract.TypedValue{row(number(1.5)), row(number(1.2))}
	diff := CompareTypedRows(columns, columns, expected, actual, RowOptions{Ordering: OrderUnordered, NumericTolerance: "0.5"})
	require.True(t, diff.Match, diff.Message)
	diff = CompareTypedRows(columns, columns, expected, actual, RowOptions{Ordering: OrderUnordered, NumericTolerance: "0.2"})
	require.False(t, diff.Match)
}

func TestCompareTypedRowsTreatsExplicitZeroToleranceAsDeclared(t *testing.T) {
	expectedInteger := columnsOf("integer")
	actualReal := columnsOf("number")
	expectedRows := [][]apicontract.TypedValue{row(integer("2"))}
	actualRows := [][]apicontract.TypedValue{row(number(2))}

	// An omitted tolerance stays strict about the SQLite storage class.
	strict := CompareTypedRows(expectedInteger, actualReal, expectedRows, actualRows, RowOptions{Ordering: OrderUnordered})
	require.False(t, strict.Match)
	require.Equal(t, ProblemColumnType, strict.Problem)

	// An explicit "0" is a declared tolerance, not an omitted one.
	parsed, err := parseTolerance("0")
	require.NoError(t, err)
	require.NotNil(t, parsed, "an explicit zero tolerance must not be normalized to absent")
	require.Zero(t, parsed.Sign())

	for _, ordering := range []string{OrderUnordered, OrderOrdered} {
		t.Run(ordering, func(t *testing.T) {
			diff := CompareTypedRows(expectedInteger, actualReal, expectedRows, actualRows, RowOptions{Ordering: ordering, NumericTolerance: "0"})
			require.True(t, diff.Match, diff.Message)
		})
	}

	// Mixed INTEGER/REAL rows in one unordered result set.
	mixedExpected := [][]apicontract.TypedValue{row(integer("1")), row(integer("2")), row(integer("3"))}
	mixedActual := [][]apicontract.TypedValue{row(number(3)), row(number(1)), row(number(2))}
	require.True(t, CompareTypedRows(expectedInteger, actualReal, mixedExpected, mixedActual, RowOptions{Ordering: OrderUnordered, NumericTolerance: "0"}).Match)
	require.False(t, CompareTypedRows(expectedInteger, actualReal, mixedExpected, mixedActual, RowOptions{Ordering: OrderUnordered}).Match)

	// Zero tolerance permits no numeric difference at all.
	different := [][]apicontract.TypedValue{row(number(2.5))}
	diff := CompareTypedRows(expectedInteger, actualReal, expectedRows, different, RowOptions{Ordering: OrderUnordered, NumericTolerance: "0"})
	require.False(t, diff.Match)

	// Canonical decimal 2.0 remains numerically equal to integer 2 under "0".
	expectedDecimal := columnsOf("decimal")
	expectedDecimalRows := [][]apicontract.TypedValue{row(apicontract.NewDecimalValue("2.0"))}
	require.True(t, CompareTypedRows(expectedDecimal, expectedInteger, expectedDecimalRows, expectedRows, RowOptions{Ordering: OrderUnordered, NumericTolerance: "0"}).Match)
}

func TestCompareTypedRowsEnforcesColumnCountNamesAndValues(t *testing.T) {
	two := columnsOf("integer", "string")
	three := columnsOf("integer", "string", "string")
	rows := [][]apicontract.TypedValue{row(integer("1"), text("x"))}

	diff := CompareTypedRows(two, three, rows, [][]apicontract.TypedValue{append(rows[0], text("extra"))}, RowOptions{Ordering: OrderUnordered})
	require.False(t, diff.Match)
	require.Equal(t, ProblemColumnCount, diff.Problem)

	aliased := []apicontract.Column{{Name: "A", Type: "integer"}, {Name: "B", Type: "string"}}
	require.True(t, CompareTypedRows(two, aliased, rows, rows, RowOptions{Ordering: OrderUnordered}).Match)

	// Duplicate observed aliases are irrelevant when names are not asserted.
	duplicateAliases := []apicontract.Column{{Name: "A", Type: "integer"}, {Name: "A", Type: "string"}}
	require.True(t, CompareTypedRows(two, duplicateAliases, rows, rows, RowOptions{Ordering: OrderUnordered}).Match)
	diff = CompareTypedRows(two, aliased, rows, rows, RowOptions{Ordering: OrderUnordered, RequireColumnNames: true})
	require.False(t, diff.Match)
	require.Equal(t, ProblemColumnName, diff.Problem)

	wrongSameCount := [][]apicontract.TypedValue{row(integer("1"), text("y"))}
	diff = CompareTypedRows(two, aliased, rows, wrongSameCount, RowOptions{Ordering: OrderUnordered})
	require.False(t, diff.Match)
	require.Equal(t, ProblemRowContent, diff.Problem)
	require.Equal(t, 1, diff.Missing)
	require.Equal(t, 1, diff.Unexpected)

	malformed := [][]apicontract.TypedValue{row(number(1), text("x"))}
	diff = CompareTypedRows(two, two, rows, malformed, RowOptions{Ordering: OrderUnordered})
	require.False(t, diff.Match)
	require.Equal(t, ProblemRowValueType, diff.Problem)

	diff = CompareTypedRows(two, two, rows, rows, RowOptions{Ordering: "sideways"})
	require.False(t, diff.Match)
	require.Equal(t, ProblemInvalidOptions, diff.Problem)
}
