package recordsetcompare

import (
	"errors"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/require"
)

func orderedRecordset(columns []apicontract.Column, rows ...[]apicontract.TypedValue) OrderedRecordset {
	return OrderedRecordset{Columns: columns, Rows: func(yield func([]apicontract.TypedValue, error) bool) {
		for _, row := range rows {
			if !yield(row, nil) {
				return
			}
		}
	}}
}

func TestCompareOrderedAdvancesLaggingCursorAndEmitsEveryState(t *testing.T) {
	columns := []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "value", Type: "string"}}
	left := orderedRecordset(columns,
		[]apicontract.TypedValue{apicontract.NewIntegerValue("1"), apicontract.NewStringValue("removed")},
		[]apicontract.TypedValue{apicontract.NewIntegerValue("3"), apicontract.NewStringValue("same")},
		[]apicontract.TypedValue{apicontract.NewIntegerValue("4"), apicontract.NewStringValue("old")},
	)
	right := orderedRecordset(columns,
		[]apicontract.TypedValue{apicontract.NewIntegerValue("2"), apicontract.NewStringValue("added")},
		[]apicontract.TypedValue{apicontract.NewIntegerValue("3"), apicontract.NewStringValue("same")},
		[]apicontract.TypedValue{apicontract.NewIntegerValue("4"), apicontract.NewStringValue("new")},
	)
	var observed []ComparedRecord
	result, err := CompareOrdered(left, right, receipt("left", 3), receipt("right", 3), Options{
		Key: []string{"id"}, Observer: func(item ComparedRecord) error {
			observed = append(observed, item)
			return nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, []RecordState{RecordRemoved, RecordAdded, RecordMatched, RecordChanged}, []RecordState{
		observed[0].State, observed[1].State, observed[2].State, observed[3].State,
	})
	for i, item := range observed {
		require.Equal(t, uint64(i), item.Ordinal)
		require.NotEmpty(t, item.SortKey)
		require.Len(t, item.Key, 1)
	}
	require.Equal(t, []apicontract.TypedValue{apicontract.NewIntegerValue("3"), apicontract.NewStringValue("same")}, observed[2].Row)
	require.Empty(t, observed[2].Deltas, "matched row is stored once without a duplicate candidate payload")
	require.Equal(t, []apicontract.TypedValue{apicontract.NewIntegerValue("4"), apicontract.NewStringValue("old")}, observed[3].Row)
	require.Equal(t, []RecordDelta{{Column: "value", Value: apicontract.NewStringValue("new")}}, observed[3].Deltas)
	require.Equal(t, apicontract.CompareSummary{Added: 1, Removed: 1, Changed: 1, Unchanged: 1, ColumnsOnlyOnOneSide: []apicontract.CompareOneSidedColumn{}}, result.Summary)
}

func TestCompareOrderedCompositeKeys(t *testing.T) {
	columns := []apicontract.Column{{Name: "tenant", Type: "string"}, {Name: "id", Type: "integer"}, {Name: "value", Type: "string"}}
	left := orderedRecordset(columns,
		[]apicontract.TypedValue{apicontract.NewStringValue("a"), apicontract.NewIntegerValue("1"), apicontract.NewStringValue("same")},
		[]apicontract.TypedValue{apicontract.NewStringValue("b"), apicontract.NewIntegerValue("1"), apicontract.NewStringValue("same")},
	)
	right := orderedRecordset(columns,
		[]apicontract.TypedValue{apicontract.NewStringValue("a"), apicontract.NewIntegerValue("1"), apicontract.NewStringValue("same")},
		[]apicontract.TypedValue{apicontract.NewStringValue("a"), apicontract.NewIntegerValue("2"), apicontract.NewStringValue("added")},
		[]apicontract.TypedValue{apicontract.NewStringValue("b"), apicontract.NewIntegerValue("1"), apicontract.NewStringValue("same")},
	)
	result, err := CompareOrdered(left, right, receipt("left", 2), receipt("right", 3), Options{Key: []string{"tenant", "id"}})
	require.NoError(t, err)
	require.Equal(t, 1, result.Summary.Added)
	require.Equal(t, 2, result.Summary.Unchanged)
	require.Equal(t, []apicontract.TypedValue{apicontract.NewStringValue("a"), apicontract.NewIntegerValue("2")}, result.Added[0].Key)
}

func TestCompareOrderedRejectsUnsortedAndDuplicateStreams(t *testing.T) {
	columns := []apicontract.Column{{Name: "id", Type: "integer"}}
	row1 := []apicontract.TypedValue{apicontract.NewIntegerValue("1")}
	row2 := []apicontract.TypedValue{apicontract.NewIntegerValue("2")}
	empty := orderedRecordset(columns)

	_, err := CompareOrdered(orderedRecordset(columns, row2, row1), empty, receipt("left", 2), receipt("right", 0), Options{Key: []string{"id"}})
	require.ErrorContains(t, err, "not strictly ascending")
	_, err = CompareOrdered(orderedRecordset(columns, row1, row1), empty, receipt("left", 2), receipt("right", 0), Options{Key: []string{"id"}})
	require.ErrorContains(t, err, "duplicate key")
}

func TestCompareOrderedObserverErrorStopsAndClosesBothInputs(t *testing.T) {
	columns := []apicontract.Column{{Name: "id", Type: "integer"}}
	closed := [2]bool{}
	stream := func(side int) OrderedRows {
		return func(yield func([]apicontract.TypedValue, error) bool) {
			defer func() { closed[side] = true }()
			for i := 1; i <= 3; i++ {
				if !yield([]apicontract.TypedValue{apicontract.NewIntegerValue(string(rune('0' + i)))}, nil) {
					return
				}
			}
		}
	}
	sinkErr := errors.New("sink transaction failed")
	_, err := CompareOrdered(
		OrderedRecordset{Columns: columns, Rows: stream(0)},
		OrderedRecordset{Columns: columns, Rows: stream(1)},
		receipt("left", 3), receipt("right", 3),
		Options{Key: []string{"id"}, Observer: func(ComparedRecord) error { return sinkErr }},
	)
	require.ErrorIs(t, err, sinkErr)
	require.Equal(t, [2]bool{true, true}, closed)
}

func TestCompareOrderedSourceErrorStopsAndClosesBothInputs(t *testing.T) {
	columns := []apicontract.Column{{Name: "id", Type: "integer"}}
	closed := [2]bool{}
	sourceErr := errors.New("reader failed")
	left := OrderedRows(func(yield func([]apicontract.TypedValue, error) bool) {
		defer func() { closed[0] = true }()
		if !yield([]apicontract.TypedValue{apicontract.NewIntegerValue("1")}, nil) {
			return
		}
		yield(nil, sourceErr)
	})
	right := OrderedRows(func(yield func([]apicontract.TypedValue, error) bool) {
		defer func() { closed[1] = true }()
		for _, id := range []string{"1", "2", "3"} {
			if !yield([]apicontract.TypedValue{apicontract.NewIntegerValue(id)}, nil) {
				return
			}
		}
	})
	_, err := CompareOrdered(
		OrderedRecordset{Columns: columns, Rows: left}, OrderedRecordset{Columns: columns, Rows: right},
		receipt("left", 1), receipt("right", 3), Options{Key: []string{"id"}},
	)
	require.ErrorIs(t, err, sourceErr)
	require.Equal(t, [2]bool{true, true}, closed)
}

func TestCompareOrderedReceiptMismatchFollowsFullConsumptionAndClosesInputs(t *testing.T) {
	columns := []apicontract.Column{{Name: "id", Type: "integer"}}
	closed := [2]bool{}
	stream := func(side int) OrderedRows {
		return func(yield func([]apicontract.TypedValue, error) bool) {
			defer func() { closed[side] = true }()
			yield([]apicontract.TypedValue{apicontract.NewIntegerValue("1")}, nil)
		}
	}
	_, err := CompareOrdered(
		OrderedRecordset{Columns: columns, Rows: stream(0)}, OrderedRecordset{Columns: columns, Rows: stream(1)},
		receipt("left", 2), receipt("right", 1), Options{Key: []string{"id"}},
	)
	require.ErrorContains(t, err, "receipt rowCount")
	require.Equal(t, [2]bool{true, true}, closed)
}

func TestCompareOrderedMatchesMaterializedCompare(t *testing.T) {
	columns := []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "state", Type: "string"}}
	left := recordset(columns,
		[]apicontract.TypedValue{apicontract.NewIntegerValue("3"), apicontract.NewStringValue("removed")},
		[]apicontract.TypedValue{apicontract.NewIntegerValue("1"), apicontract.NewStringValue("old")},
		[]apicontract.TypedValue{apicontract.NewIntegerValue("2"), apicontract.NewStringValue("same")},
	)
	right := recordset(columns,
		[]apicontract.TypedValue{apicontract.NewIntegerValue("4"), apicontract.NewStringValue("added")},
		[]apicontract.TypedValue{apicontract.NewIntegerValue("2"), apicontract.NewStringValue("same")},
		[]apicontract.TypedValue{apicontract.NewIntegerValue("1"), apicontract.NewStringValue("new")},
	)
	options := Options{Key: []string{"id"}, DistributionColumn: "state", Limit: 2}
	want, err := Compare(left, right, receipt("left", 3), receipt("right", 3), options)
	require.NoError(t, err)
	got, err := CompareOrdered(
		orderedRecordset(columns, left.Rows[1], left.Rows[2], left.Rows[0]),
		orderedRecordset(columns, right.Rows[2], right.Rows[1], right.Rows[0]),
		receipt("left", 3), receipt("right", 3), options,
	)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestCompareOrderedObserverNeverSeesHiddenOrOneSidedColumns(t *testing.T) {
	leftColumns := []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "left_only", Type: "string"}, {Name: "secret", Type: "string"}}
	rightColumns := []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "right_only", Type: "string"}, {Name: "secret", Type: "string"}}
	leftReceipt := receipt("left", 1)
	leftReceipt.Limitations = []apicontract.Limitation{{Policy: "masked", HiddenColumns: []string{"secret"}}}
	var observed ComparedRecord
	_, err := CompareOrdered(
		orderedRecordset(leftColumns, []apicontract.TypedValue{apicontract.NewIntegerValue("1"), apicontract.NewStringValue("left"), apicontract.NewStringValue("top-secret")}),
		orderedRecordset(rightColumns, []apicontract.TypedValue{apicontract.NewIntegerValue("1"), apicontract.NewStringValue("right"), apicontract.NewStringValue("different-secret")}),
		leftReceipt, receipt("right", 1),
		Options{Key: []string{"id"}, Observer: func(item ComparedRecord) error { observed = item; return nil }},
	)
	require.NoError(t, err)
	require.Equal(t, RecordMatched, observed.State)
	require.Equal(t, []apicontract.TypedValue{apicontract.NewIntegerValue("1")}, observed.Row)
}
