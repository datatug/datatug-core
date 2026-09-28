package recordsetcompare

import (
	"errors"
	"fmt"
	"testing"

	"github.com/dal-go/dalgo/recordops"
	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompare_ValidationErrors(t *testing.T) {
	rcpt := receipt("1", 1)
	cols := []apicontract.Column{{Name: "id", Type: "integer"}}
	rs := apicontract.Recordset{
		Columns: cols,
		Rows:    [][]apicontract.TypedValue{{apicontract.NewIntegerValue("1")}},
	}

	t.Run("left_receipt_invalid", func(t *testing.T) {
		bad := rcpt
		bad.RowCount = -1
		_, err := Compare(rs, rs, bad, rcpt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "left receipt")
	})

	t.Run("right_receipt_invalid", func(t *testing.T) {
		bad := rcpt
		bad.RowCount = -1
		_, err := Compare(rs, rs, rcpt, bad, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "right receipt")
	})

	t.Run("left_rs_invalid", func(t *testing.T) {
		bad := rs
		bad.Columns = []apicontract.Column{{Name: "", Type: "integer"}}
		_, err := Compare(bad, rs, rcpt, rcpt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "left recordset")
	})

	t.Run("right_rs_invalid", func(t *testing.T) {
		bad := rs
		bad.Columns = []apicontract.Column{{Name: "", Type: "integer"}}
		_, err := Compare(rs, bad, rcpt, rcpt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "right recordset")
	})

	t.Run("left_column_type_mismatch", func(t *testing.T) {
		bad := apicontract.Recordset{
			Columns: cols,
			Rows:    [][]apicontract.TypedValue{{apicontract.NewStringValue("abc")}},
		}
		_, err := Compare(bad, rs, rcpt, rcpt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "left recordset: row 0 column 0 does not match declared column type")
	})

	t.Run("right_column_type_mismatch", func(t *testing.T) {
		bad := apicontract.Recordset{
			Columns: cols,
			Rows:    [][]apicontract.TypedValue{{apicontract.NewStringValue("abc")}},
		}
		_, err := Compare(rs, bad, rcpt, rcpt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "right recordset: row 0 column 0 does not match declared column type")
	})

	t.Run("receipt_rowcount_mismatch", func(t *testing.T) {
		badReceipt := rcpt
		badReceipt.RowCount = 99
		_, err := Compare(rs, rs, badReceipt, rcpt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "receipt rowCount must match the supplied recordset")
	})

	t.Run("materializedOrderedRows_right_error", func(t *testing.T) {
		badRight := apicontract.Recordset{
			Columns: cols,
			Rows:    [][]apicontract.TypedValue{{apicontract.NewNullValue()}},
		}
		badReceipt := rcpt
		badReceipt.RowCount = 1
		_, err := Compare(rs, badRight, rcpt, badReceipt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "right recordset: row 0 has null/missing key column")
	})
}

func TestCompare_HelpersCoverage(t *testing.T) {
	t.Run("indexedColumns_duplicate", func(t *testing.T) {
		_, err := columnsByName([]apicontract.Column{{Name: "id", Type: "integer"}, {Name: "id", Type: "integer"}}, nil)
		assert.ErrorContains(t, err, "duplicate column \"id\"")
	})

	t.Run("intersectColumns_hidden_and_incompatible", func(t *testing.T) {
		leftCols := []apicontract.Column{
			{Name: "col1", Type: "string"},
			{Name: "hidden1", Type: "string"},
			{Name: "only_left", Type: "string"},
		}
		rightCols := []apicontract.Column{
			{Name: "col1", Type: "integer"}, // incompatible type
			{Name: "hidden2", Type: "string"},
			{Name: "only_right", Type: "string"},
		}
		hidden := map[string]bool{"hidden1": true, "hidden2": true}
		leftMap, err := columnsByName(leftCols, hidden)
		require.NoError(t, err)
		rightMap, err := columnsByName(rightCols, hidden)
		require.NoError(t, err)

		_, _, err = intersectColumns(leftCols, rightCols, leftMap, rightMap, hidden)
		assert.ErrorContains(t, err, "incompatible types")
	})

	t.Run("intersectColumns_hidden_branches", func(t *testing.T) {
		hidden := map[string]bool{"h": true}
		shared, oneSided, err := intersectColumns(nil, nil, map[string]int{"h": 0}, map[string]int{"h": 0}, hidden)
		require.NoError(t, err)
		assert.Empty(t, shared)
		assert.Empty(t, oneSided)
	})

	t.Run("encodeTuple_number_zero", func(t *testing.T) {
		res := encodeTuple([]apicontract.TypedValue{
			apicontract.NewNumberValue(0),
		})
		assert.NotEmpty(t, res)
	})

	t.Run("percentage_zero", func(t *testing.T) {
		assert.Equal(t, 0.0, percentage(5, 0))
	})
}

func TestCompareOrdered_ValidationErrors(t *testing.T) {
	rcpt := receipt("1", 0)
	cols := []apicontract.Column{{Name: "id", Type: "integer"}}
	emptyStream := func(yield func([]apicontract.TypedValue, error) bool) {}

	t.Run("left_rows_nil", func(t *testing.T) {
		_, err := CompareOrdered(OrderedRecordset{Columns: cols}, OrderedRecordset{Columns: cols, Rows: emptyStream}, rcpt, rcpt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "left ordered recordset: rows stream is required")
	})

	t.Run("right_rows_nil", func(t *testing.T) {
		_, err := CompareOrdered(OrderedRecordset{Columns: cols, Rows: emptyStream}, OrderedRecordset{Columns: cols}, rcpt, rcpt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "right ordered recordset: rows stream is required")
	})

	t.Run("left_columns_invalid", func(t *testing.T) {
		badCols := []apicontract.Column{{Name: "", Type: "integer"}}
		_, err := CompareOrdered(OrderedRecordset{Columns: badCols, Rows: emptyStream}, OrderedRecordset{Columns: cols, Rows: emptyStream}, rcpt, rcpt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "left ordered recordset")
	})

	t.Run("right_columns_invalid", func(t *testing.T) {
		badCols := []apicontract.Column{{Name: "", Type: "integer"}}
		_, err := CompareOrdered(OrderedRecordset{Columns: cols, Rows: emptyStream}, OrderedRecordset{Columns: badCols, Rows: emptyStream}, rcpt, rcpt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "right ordered recordset")
	})

	t.Run("prepareComparison_error", func(t *testing.T) {
		_, err := CompareOrdered(OrderedRecordset{Columns: cols, Rows: emptyStream}, OrderedRecordset{Columns: cols, Rows: emptyStream}, rcpt, rcpt, Options{Key: nil})
		assert.ErrorContains(t, err, "recordsetcompare: key is required")
	})
}

func TestPrepareComparison_Errors(t *testing.T) {
	rcpt := receipt("1", 0)
	cols := []apicontract.Column{{Name: "id", Type: "integer"}}

	t.Run("left_receipt_invalid", func(t *testing.T) {
		bad := rcpt
		bad.RowCount = -1
		_, err := prepareComparison(cols, cols, bad, rcpt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "left receipt")
	})

	t.Run("right_receipt_invalid", func(t *testing.T) {
		bad := rcpt
		bad.RowCount = -1
		_, err := prepareComparison(cols, cols, rcpt, bad, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "right receipt")
	})

	t.Run("limit_invalid", func(t *testing.T) {
		_, err := prepareComparison(cols, cols, rcpt, rcpt, Options{Key: []string{"id"}, Limit: -1})
		assert.ErrorContains(t, err, "limit must be between 1 and")
	})

	t.Run("left_columnsByName_error", func(t *testing.T) {
		badCols := []apicontract.Column{{Name: "id", Type: "int"}, {Name: "id", Type: "int"}}
		_, err := prepareComparison(badCols, cols, rcpt, rcpt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "left recordset: duplicate column")
	})

	t.Run("right_columnsByName_error", func(t *testing.T) {
		badCols := []apicontract.Column{{Name: "id", Type: "int"}, {Name: "id", Type: "int"}}
		_, err := prepareComparison(cols, badCols, rcpt, rcpt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "right recordset: duplicate column")
	})

	t.Run("intersectColumns_error", func(t *testing.T) {
		rightCols := []apicontract.Column{{Name: "id", Type: "string"}}
		_, err := prepareComparison(cols, rightCols, rcpt, rcpt, Options{Key: []string{"id"}})
		assert.ErrorContains(t, err, "incompatible types")
	})

	t.Run("duplicate_key_column", func(t *testing.T) {
		_, err := prepareComparison(cols, cols, rcpt, rcpt, Options{Key: []string{"id", "id"}})
		assert.ErrorContains(t, err, "duplicate key column \"id\"")
	})

	t.Run("distribution_column_missing_from_shared", func(t *testing.T) {
		_, err := prepareComparison(cols, cols, rcpt, rcpt, Options{Key: []string{"id"}, DistributionColumn: "nonexistent"})
		assert.ErrorContains(t, err, "distribution column \"nonexistent\" is missing from the shared columns")
	})
}

func TestStream_OrderedRowsToRecords_Validation(t *testing.T) {
	cols := []apicontract.Column{{Name: "id", Type: "integer"}}
	plan := comparisonPlan{
		keyNames:    []string{"id"},
		sharedNames: []string{"id"},
	}
	indexes := map[string]int{"id": 0}

	t.Run("row_length_mismatch", func(t *testing.T) {
		rows := func(yield func([]apicontract.TypedValue, error) bool) {
			yield([]apicontract.TypedValue{}, nil)
		}
		var rowCount int
		seq := orderedRowsToRecords("left", rows, cols, indexes, plan, 0, &rowCount, nil)
		for _, err := range seq {
			assert.ErrorContains(t, err, "has 0 values, want 1")
		}
	})

	t.Run("value_validate_error", func(t *testing.T) {
		rows := func(yield func([]apicontract.TypedValue, error) bool) {
			yield([]apicontract.TypedValue{{Type: apicontract.ValueType("bad_type")}}, nil)
		}
		var rowCount int
		seq := orderedRowsToRecords("left", rows, cols, indexes, plan, 0, &rowCount, nil)
		for _, err := range seq {
			assert.ErrorContains(t, err, "row 0 column 0")
		}
	})

	t.Run("value_type_mismatch", func(t *testing.T) {
		rows := func(yield func([]apicontract.TypedValue, error) bool) {
			yield([]apicontract.TypedValue{apicontract.NewStringValue("abc")}, nil)
		}
		var rowCount int
		seq := orderedRowsToRecords("left", rows, cols, indexes, plan, 0, &rowCount, nil)
		for _, err := range seq {
			assert.ErrorContains(t, err, "does not match declared column type")
		}
	})

	t.Run("null_key_column", func(t *testing.T) {
		rows := func(yield func([]apicontract.TypedValue, error) bool) {
			yield([]apicontract.TypedValue{apicontract.NewNullValue()}, nil)
		}
		var rowCount int
		seq := orderedRowsToRecords("left", rows, cols, indexes, plan, 0, &rowCount, nil)
		for _, err := range seq {
			assert.ErrorContains(t, err, "has null/missing key column")
		}
	})

	t.Run("typedKeySortKey_error", func(t *testing.T) {
		emptyKeyPlan := comparisonPlan{
			keyNames:    []string{},
			sharedNames: []string{"id"},
		}
		rows := func(yield func([]apicontract.TypedValue, error) bool) {
			yield([]apicontract.TypedValue{apicontract.NewIntegerValue("1")}, nil)
		}
		var rowCount int
		seq := orderedRowsToRecords("left", rows, cols, indexes, emptyKeyPlan, 0, &rowCount, nil)
		for _, err := range seq {
			assert.ErrorContains(t, err, "key:")
		}
	})
}

func TestStream_ClassifyDiff_Branches(t *testing.T) {
	plan := comparisonPlan{
		keyNames:    []string{"id"},
		sharedNames: []string{"id", "val"},
	}

	t.Run("candidates_not_1", func(t *testing.T) {
		diff := recordops.IDDiff[string]{
			Candidates: []recordops.CandidateState{},
		}
		_, _, err := classifyDiff(diff, 0, plan)
		assert.ErrorContains(t, err, "returned 0 candidates, want 1")
	})

	t.Run("missing_baseline_nil", func(t *testing.T) {
		diff := recordops.IDDiff[string]{
			Candidates: []recordops.CandidateState{{Status: recordops.Missing}},
			Baseline:   nil,
		}
		_, _, err := classifyDiff(diff, 0, plan)
		assert.ErrorContains(t, err, "missing right row has no left baseline")
	})

	t.Run("matched_baseline_nil", func(t *testing.T) {
		diff := recordops.IDDiff[string]{
			Candidates: []recordops.CandidateState{{Status: recordops.Matched}},
			Baseline:   nil,
		}
		_, _, err := classifyDiff(diff, 0, plan)
		assert.ErrorContains(t, err, "matched row has no left baseline")
	})

	t.Run("changed_baseline_nil", func(t *testing.T) {
		diff := recordops.IDDiff[string]{
			Candidates: []recordops.CandidateState{{Status: recordops.Changed}},
			Baseline:   nil,
		}
		_, _, err := classifyDiff(diff, 0, plan)
		assert.ErrorContains(t, err, "changed row has no left baseline")
	})

	t.Run("unknown_status", func(t *testing.T) {
		diff := recordops.IDDiff[string]{
			Candidates: []recordops.CandidateState{{Status: recordops.RecordStatus(99)}},
		}
		_, _, err := classifyDiff(diff, 0, plan)
		assert.ErrorContains(t, err, "unknown DALgo record status")
	})

	t.Run("valuesFromFields_error_wrapped", func(t *testing.T) {
		diff := recordops.IDDiff[string]{
			Candidates: []recordops.CandidateState{
				{Status: recordops.Extra, Fields: []recordops.FieldValue{{Name: "id", Absent: true}}},
			},
		}
		_, _, err := classifyDiff(diff, 0, plan)
		assert.ErrorContains(t, err, "shared field \"id\" is absent")
	})
}

func TestStream_ValuesFromFields(t *testing.T) {
	names := []string{"a", "b"}

	t.Run("field_absent", func(t *testing.T) {
		fields := []recordops.FieldValue{{Name: "a", Absent: true}}
		_, err := valuesFromFields(fields, names)
		assert.ErrorContains(t, err, "shared field \"a\" is absent")
	})

	t.Run("field_unexpected_type", func(t *testing.T) {
		fields := []recordops.FieldValue{{Name: "a", Value: 123}}
		_, err := valuesFromFields(fields, names)
		assert.ErrorContains(t, err, "unexpected value type int")
	})

	t.Run("duplicate_field", func(t *testing.T) {
		val := apicontract.NewIntegerValue("1")
		fields := []recordops.FieldValue{{Name: "a", Value: val}, {Name: "a", Value: val}}
		_, err := valuesFromFields(fields, names)
		assert.ErrorContains(t, err, "duplicate shared field \"a\"")
	})

	t.Run("missing_field", func(t *testing.T) {
		val := apicontract.NewIntegerValue("1")
		fields := []recordops.FieldValue{{Name: "a", Value: val}}
		_, err := valuesFromFields(fields, names)
		assert.ErrorContains(t, err, "shared field \"b\" is missing")
	})

	t.Run("extra_field", func(t *testing.T) {
		val := apicontract.NewIntegerValue("1")
		fields := []recordops.FieldValue{
			{Name: "a", Value: val},
			{Name: "b", Value: val},
			{Name: "extra", Value: val},
		}
		_, err := valuesFromFields(fields, names)
		assert.ErrorContains(t, err, "record contains a field outside the shared-visible columns")
	})
}

func TestStream_ChangedFields(t *testing.T) {
	plan := comparisonPlan{
		sharedNames: []string{"id", "val1", "val2"},
		keySet:      map[string]bool{"id": true},
	}
	baseline := []apicontract.TypedValue{
		apicontract.NewIntegerValue("1"),
		apicontract.NewStringValue("old1"),
		apicontract.NewStringValue("old2"),
	}

	t.Run("field_outside_shared", func(t *testing.T) {
		fields := []recordops.FieldValue{{Name: "unknown"}}
		_, _, err := changedFields(baseline, fields, plan)
		assert.ErrorContains(t, err, "changed field is outside the shared-visible columns")
	})

	t.Run("key_field_changed", func(t *testing.T) {
		fields := []recordops.FieldValue{{Name: "id"}}
		_, _, err := changedFields(baseline, fields, plan)
		assert.ErrorContains(t, err, "key field \"id\" was returned as changed")
	})

	t.Run("duplicate_changed_field", func(t *testing.T) {
		val := apicontract.NewStringValue("new")
		fields := []recordops.FieldValue{{Name: "val1", Value: val}, {Name: "val1", Value: val}}
		_, _, err := changedFields(baseline, fields, plan)
		assert.ErrorContains(t, err, "duplicate changed field \"val1\"")
	})

	t.Run("changed_field_absent", func(t *testing.T) {
		fields := []recordops.FieldValue{{Name: "val1", Absent: true}}
		_, _, err := changedFields(baseline, fields, plan)
		assert.ErrorContains(t, err, "shared field \"val1\" is absent from right row")
	})

	t.Run("changed_field_unexpected_type", func(t *testing.T) {
		fields := []recordops.FieldValue{{Name: "val1", Value: 123}}
		_, _, err := changedFields(baseline, fields, plan)
		assert.ErrorContains(t, err, "unexpected value type int")
	})

	t.Run("multiple_changed_fields_sort", func(t *testing.T) {
		valZ := apicontract.NewStringValue("z")
		valA := apicontract.NewStringValue("a")
		fields := []recordops.FieldValue{
			{Name: "val2", Value: valZ},
			{Name: "val1", Value: valA},
		}
		changes, deltas, err := changedFields(baseline, fields, plan)
		require.NoError(t, err)
		assert.Equal(t, "val1", changes[0].Column)
		assert.Equal(t, "val2", changes[1].Column)
		assert.Equal(t, "val1", deltas[0].Column)
		assert.Equal(t, "val2", deltas[1].Column)
	})
}

func TestStream_CompareOrderedPrepared_ClassifyHooks(t *testing.T) {
	rcpt := receipt("1", 1)
	cols := []apicontract.Column{{Name: "id", Type: "integer"}}
	plan := comparisonPlan{
		leftColumns:  map[string]int{"id": 0},
		rightColumns: map[string]int{"id": 0},
		shared:       cols,
		sharedNames:  []string{"id"},
		keyNames:     []string{"id"},
		keySet:       map[string]bool{"id": true},
		leftReceipt:  rcpt,
		rightReceipt: rcpt,
		limit:        100,
	}

	stream := func(yield func([]apicontract.TypedValue, error) bool) {
		yield([]apicontract.TypedValue{apicontract.NewIntegerValue("1")}, nil)
	}
	left := OrderedRecordset{Columns: cols, Rows: stream}
	right := OrderedRecordset{Columns: cols, Rows: stream}

	oldHook := classifyDiffFunc
	defer func() { classifyDiffFunc = oldHook }()

	t.Run("classify_error", func(t *testing.T) {
		classifyDiffFunc = func(diff recordops.IDDiff[string], ordinal uint64, plan comparisonPlan) (ComparedRecord, []apicontract.CompareColumnChange, error) {
			return ComparedRecord{}, nil, errors.New("classify fail")
		}
		_, err := compareOrderedPrepared(left, right, plan)
		assert.ErrorContains(t, err, "classify fail")
	})

	t.Run("unsupported_record_state", func(t *testing.T) {
		classifyDiffFunc = func(diff recordops.IDDiff[string], ordinal uint64, plan comparisonPlan) (ComparedRecord, []apicontract.CompareColumnChange, error) {
			return ComparedRecord{State: "INVALID_STATE"}, nil, nil
		}
		_, err := compareOrderedPrepared(left, right, plan)
		assert.ErrorContains(t, err, "unsupported record state")
	})

	t.Run("result_validate_failure", func(t *testing.T) {
		classifyDiffFunc = func(diff recordops.IDDiff[string], ordinal uint64, plan comparisonPlan) (ComparedRecord, []apicontract.CompareColumnChange, error) {
			return ComparedRecord{
				State: RecordAdded,
				Key:   nil, // invalid: empty key violates CompareRow.Validate()
				Row:   []apicontract.TypedValue{apicontract.NewIntegerValue("1")},
			}, nil, nil
		}
		_, err := compareOrderedPrepared(left, right, plan)
		assert.ErrorContains(t, err, "invalid result")
	})
}

func TestStream_DistributionTracker_CapacityEviction(t *testing.T) {
	tracker := newDistributionTracker("col")
	for i := 0; i < apicontract.CompareDistributionMaximumValues; i++ {
		tracker.add(apicontract.NewStringValue(fmt.Sprintf("val-%04d", i)), true)
	}
	assert.Len(t, tracker.entries, apicontract.CompareDistributionMaximumValues)

	// Add a larger value that is ignored
	tracker.add(apicontract.NewStringValue("val-9999-larger"), true)
	assert.True(t, tracker.truncated)

	// Add a smaller value that replaces the largest
	tracker.add(apicontract.NewStringValue("val-0000-small"), true)
	assert.True(t, tracker.truncated)
}

func TestStream_CompareDistributionValue(t *testing.T) {
	// Fallback to identity comparison when keys match or sort keys cannot be extracted
	val1 := apicontract.NewStringValue("same")
	val2 := apicontract.NewStringValue("same")

	order1 := compareDistributionValue(val1, "identityA", val2, "identityB")
	assert.Negative(t, order1)

	order2 := compareDistributionValue(val1, "identityA", val2, "identityA")
	assert.Zero(t, order2)
}

func TestMaterializedOrderedRows_TypedKeySortKeyError(t *testing.T) {
	cols := []apicontract.Column{{Name: "id", Type: "integer"}}
	rs := apicontract.Recordset{
		Columns: cols,
		Rows: [][]apicontract.TypedValue{
			{apicontract.NewIntegerValue("1")},
		},
	}
	_, err := materializedOrderedRows(rs, map[string]int{"id": 0}, []string{})
	assert.ErrorContains(t, err, "key:")
}
