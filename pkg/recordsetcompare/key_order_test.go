package recordsetcompare

import (
	"math"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/require"
)

func TestTypedKeyNaturalOrderAndSortableEncoding(t *testing.T) {
	tests := []struct {
		name   string
		values []apicontract.TypedValue
	}{
		{"number", []apicontract.TypedValue{
			apicontract.NewNumberValue(-10), apicontract.NewNumberValue(-2), apicontract.NewNumberValue(0),
			apicontract.NewNumberValue(2), apicontract.NewNumberValue(10),
		}},
		{"integer", []apicontract.TypedValue{
			apicontract.NewIntegerValue("-10"), apicontract.NewIntegerValue("-2"), apicontract.NewIntegerValue("0"),
			apicontract.NewIntegerValue("1"), apicontract.NewIntegerValue("2"), apicontract.NewIntegerValue("10"),
		}},
		{"decimal", []apicontract.TypedValue{
			apicontract.NewDecimalValue("-10"), apicontract.NewDecimalValue("-2"), apicontract.NewDecimalValue("-1.2"),
			apicontract.NewDecimalValue("-1.20"), apicontract.NewDecimalValue("-0.0"), apicontract.NewDecimalValue("0"),
			apicontract.NewDecimalValue("0.0"), apicontract.NewDecimalValue("0.01"), apicontract.NewDecimalValue("1.2"),
			apicontract.NewDecimalValue("1.20"), apicontract.NewDecimalValue("10"),
		}},
		{"boolean", []apicontract.TypedValue{apicontract.NewBooleanValue(false), apicontract.NewBooleanValue(true)}},
		{"date", []apicontract.TypedValue{
			apicontract.NewDateValue("2025-12-31"), apicontract.NewDateValue("2026-01-01"), apicontract.NewDateValue("2026-09-14"),
		}},
		{"datetime", []apicontract.TypedValue{
			apicontract.NewDatetimeValue("2026-09-14T00:00:00.000Z"),
			apicontract.NewDatetimeValue("2026-09-14T00:00:00Z"),
			apicontract.NewDatetimeValue("2026-09-14T00:00:00.1Z"),
		}},
		{"string binary", []apicontract.TypedValue{
			apicontract.NewStringValue("A"), apicontract.NewStringValue("a"), apicontract.NewStringValue("aa"), apicontract.NewStringValue("ä"),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for i := 1; i < len(test.values); i++ {
				left, right := []apicontract.TypedValue{test.values[i-1]}, []apicontract.TypedValue{test.values[i]}
				order, err := CompareTypedKeys(left, right)
				require.NoError(t, err)
				require.Negative(t, order)
				leftKey, err := TypedKeySortKey(left)
				require.NoError(t, err)
				rightKey, err := TypedKeySortKey(right)
				require.NoError(t, err)
				require.Less(t, leftKey, rightKey)
			}
		})
	}

	minusZero := []apicontract.TypedValue{apicontract.NewNumberValue(math.Copysign(0, -1))}
	plusZero := []apicontract.TypedValue{apicontract.NewNumberValue(0)}
	order, err := CompareTypedKeys(minusZero, plusZero)
	require.NoError(t, err)
	require.Zero(t, order)
	minusZeroKey, err := TypedKeySortKey(minusZero)
	require.NoError(t, err)
	plusZeroKey, err := TypedKeySortKey(plusZero)
	require.NoError(t, err)
	require.Equal(t, minusZeroKey, plusZeroKey)
}

func TestTypedCompositeKeyNaturalOrder(t *testing.T) {
	keys := [][]apicontract.TypedValue{
		{apicontract.NewStringValue("a"), apicontract.NewIntegerValue("2")},
		{apicontract.NewStringValue("a"), apicontract.NewIntegerValue("10")},
		{apicontract.NewStringValue("b"), apicontract.NewIntegerValue("-10")},
	}
	for i := 1; i < len(keys); i++ {
		order, err := CompareTypedKeys(keys[i-1], keys[i])
		require.NoError(t, err)
		require.Negative(t, order)
		left, err := TypedKeySortKey(keys[i-1])
		require.NoError(t, err)
		right, err := TypedKeySortKey(keys[i])
		require.NoError(t, err)
		require.Less(t, left, right)
	}
}

func TestCompareOrderedAcceptsNativeNumericOrder(t *testing.T) {
	columns := []apicontract.Column{{Name: "id", Type: "integer"}}
	rows := [][]apicontract.TypedValue{
		{apicontract.NewIntegerValue("1")},
		{apicontract.NewIntegerValue("2")},
		{apicontract.NewIntegerValue("10")},
	}
	result, err := CompareOrdered(
		orderedRecordset(columns, rows...), orderedRecordset(columns, rows...),
		receipt("left", 3), receipt("right", 3), Options{Key: []string{"id"}},
	)
	require.NoError(t, err)
	require.Equal(t, 3, result.Summary.Unchanged)
}

func TestCompareOrderedEmittedResultsUseNaturalNumericOrder(t *testing.T) {
	idColumns := []apicontract.Column{{Name: "id", Type: "integer"}}
	rows := [][]apicontract.TypedValue{{apicontract.NewIntegerValue("2")}, {apicontract.NewIntegerValue("10")}}
	empty := orderedRecordset(idColumns)

	t.Run("added", func(t *testing.T) {
		result, err := CompareOrdered(empty, orderedRecordset(idColumns, rows...), receipt("left", 0), receipt("right", 2), Options{Key: []string{"id"}})
		require.NoError(t, err)
		require.Equal(t, []apicontract.TypedValue{apicontract.NewIntegerValue("2")}, result.Added[0].Key)
		require.Equal(t, []apicontract.TypedValue{apicontract.NewIntegerValue("10")}, result.Added[1].Key)
	})

	t.Run("removed", func(t *testing.T) {
		result, err := CompareOrdered(orderedRecordset(idColumns, rows...), empty, receipt("left", 2), receipt("right", 0), Options{Key: []string{"id"}})
		require.NoError(t, err)
		require.Equal(t, []apicontract.TypedValue{apicontract.NewIntegerValue("2")}, result.Removed[0].Key)
		require.Equal(t, []apicontract.TypedValue{apicontract.NewIntegerValue("10")}, result.Removed[1].Key)
	})

	t.Run("changed", func(t *testing.T) {
		columns := []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "value", Type: "string"}}
		left := orderedRecordset(columns,
			[]apicontract.TypedValue{apicontract.NewIntegerValue("2"), apicontract.NewStringValue("old")},
			[]apicontract.TypedValue{apicontract.NewIntegerValue("10"), apicontract.NewStringValue("old")},
		)
		right := orderedRecordset(columns,
			[]apicontract.TypedValue{apicontract.NewIntegerValue("2"), apicontract.NewStringValue("new")},
			[]apicontract.TypedValue{apicontract.NewIntegerValue("10"), apicontract.NewStringValue("new")},
		)
		result, err := CompareOrdered(left, right, receipt("left", 2), receipt("right", 2), Options{Key: []string{"id"}})
		require.NoError(t, err)
		require.Equal(t, []apicontract.TypedValue{apicontract.NewIntegerValue("2")}, result.Changed[0].Key)
		require.Equal(t, []apicontract.TypedValue{apicontract.NewIntegerValue("10")}, result.Changed[1].Key)
	})

	t.Run("distribution", func(t *testing.T) {
		columns := []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "bucket", Type: "integer"}}
		stream := orderedRecordset(columns,
			[]apicontract.TypedValue{apicontract.NewIntegerValue("1"), apicontract.NewIntegerValue("10")},
			[]apicontract.TypedValue{apicontract.NewIntegerValue("2"), apicontract.NewNullValue()},
			[]apicontract.TypedValue{apicontract.NewIntegerValue("3"), apicontract.NewIntegerValue("2")},
		)
		result, err := CompareOrdered(stream, stream, receipt("left", 3), receipt("right", 3), Options{Key: []string{"id"}, DistributionColumn: "bucket"})
		require.NoError(t, err)
		require.Equal(t, apicontract.NewNullValue(), result.Distribution.Values[0].Value)
		require.Equal(t, apicontract.NewIntegerValue("2"), result.Distribution.Values[1].Value)
		require.Equal(t, apicontract.NewIntegerValue("10"), result.Distribution.Values[2].Value)
	})
}

func TestCompareTypedKeysRejectsMismatchedAndNullComponents(t *testing.T) {
	_, err := CompareTypedKeys(
		[]apicontract.TypedValue{apicontract.NewIntegerValue("1")},
		[]apicontract.TypedValue{apicontract.NewStringValue("1")},
	)
	require.ErrorContains(t, err, "types differ")
	_, err = TypedKeySortKey([]apicontract.TypedValue{apicontract.NewNullValue()})
	require.ErrorContains(t, err, "null key")
}
