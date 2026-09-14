package recordsetcompare

import (
	"errors"
	"fmt"
	"iter"
	"sort"
	"strings"

	"github.com/dal-go/dalgo/recordops"
	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/apicontract"
)

// OrderedRows is a single-use stream of rows ordered strictly by Options.Key
// using CompareTypedKeys (or equivalently TypedKeySortKey under binary
// collation). A source can release resources with defer: DALgo recordops stops
// both inputs when comparison completes or aborts.
type OrderedRows = iter.Seq2[[]apicontract.TypedValue, error]

// OrderedRecordset carries stable column metadata and an already key-ordered
// row stream. Unlike apicontract.Recordset, it need not materialize every row.
type OrderedRecordset struct {
	Columns []apicontract.Column
	Rows    OrderedRows
}

// RecordState classifies one aligned key from left (baseline) to right
// (candidate).
type RecordState string

const (
	RecordMatched RecordState = "matched"
	RecordAdded   RecordState = "added"
	RecordRemoved RecordState = "removed"
	RecordChanged RecordState = "changed"
)

// RecordDelta is one right-side change relative to ComparedRecord.Row. Absent
// is structurally distinct from a present null value.
type RecordDelta struct {
	Column string
	Value  apicontract.TypedValue
	Absent bool
}

// ComparedRecord is the neutral per-key stream event exposed to consumers that
// need more than the bounded API result. Row contains the full shared-visible
// row once: the matched/added/removed row, or the left baseline for changed.
// Deltas is populated only for changed rows and contains right-side values.
type ComparedRecord struct {
	Ordinal uint64
	SortKey string
	State   RecordState
	Key     []apicontract.TypedValue
	Row     []apicontract.TypedValue
	Deltas  []RecordDelta
}

// RecordObserver receives every aligned, post-policy shared-visible record in
// deterministic key order. It runs synchronously; returning an error aborts
// comparison and closes both input streams so a caller can roll back its sink.
type RecordObserver func(ComparedRecord) error

type comparisonPlan struct {
	leftColumns    map[string]int
	rightColumns   map[string]int
	shared         []apicontract.Column
	sharedNames    []string
	keyNames       []string
	keySet         map[string]bool
	oneSided       []apicontract.CompareOneSidedColumn
	leftReceipt    apicontract.CompareSideReceipt
	rightReceipt   apicontract.CompareSideReceipt
	distribution   string
	distributionAt [2]int
	limit          int
	observer       RecordObserver
}

// CompareOrdered compares two streams that are already strictly ordered by
// Options.Key using CompareTypedKeys. Alignment delegates to DALgo
// recordops.DiffFunc, which advances the cursor(s) at the smallest key and
// retains one record per input.
func CompareOrdered(left, right OrderedRecordset, leftReceipt, rightReceipt apicontract.CompareSideReceipt, options Options) (apicontract.CompareResult, error) {
	if left.Rows == nil {
		return apicontract.CompareResult{}, fmt.Errorf("left ordered recordset: rows stream is required")
	}
	if right.Rows == nil {
		return apicontract.CompareResult{}, fmt.Errorf("right ordered recordset: rows stream is required")
	}
	if err := (apicontract.Recordset{Columns: left.Columns}).Validate(); err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("left ordered recordset: %w", err)
	}
	if err := (apicontract.Recordset{Columns: right.Columns}).Validate(); err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("right ordered recordset: %w", err)
	}
	prepared, err := prepareComparison(left.Columns, right.Columns, leftReceipt, rightReceipt, options)
	if err != nil {
		return apicontract.CompareResult{}, err
	}
	return compareOrderedPrepared(left, right, prepared)
}

func prepareComparison(leftList, rightList []apicontract.Column, leftReceipt, rightReceipt apicontract.CompareSideReceipt, options Options) (comparisonPlan, error) {
	if err := leftReceipt.Validate(); err != nil {
		return comparisonPlan{}, fmt.Errorf("left receipt: %w", err)
	}
	if err := rightReceipt.Validate(); err != nil {
		return comparisonPlan{}, fmt.Errorf("right receipt: %w", err)
	}
	hidden := hiddenColumns(leftReceipt.Limitations, rightReceipt.Limitations)
	if len(options.Key) == 0 {
		return comparisonPlan{}, fmt.Errorf("recordsetcompare: key is required")
	}
	limit := options.Limit
	if limit == 0 {
		limit = apicontract.CompareDefaultLimit
	}
	if limit < 1 || limit > apicontract.CompareMaximumLimit {
		return comparisonPlan{}, fmt.Errorf("recordsetcompare: limit must be between 1 and %d", apicontract.CompareMaximumLimit)
	}
	leftColumns, err := columnsByName(leftList, hidden)
	if err != nil {
		return comparisonPlan{}, fmt.Errorf("left recordset: %w", err)
	}
	rightColumns, err := columnsByName(rightList, hidden)
	if err != nil {
		return comparisonPlan{}, fmt.Errorf("right recordset: %w", err)
	}
	shared, oneSided, err := intersectColumns(leftList, rightList, leftColumns, rightColumns, hidden)
	if err != nil {
		return comparisonPlan{}, err
	}
	sharedNames := make([]string, len(shared))
	sharedSet := make(map[string]bool, len(shared))
	for i, column := range shared {
		sharedNames[i] = column.Name
		sharedSet[column.Name] = true
	}
	keySet := make(map[string]bool, len(options.Key))
	for _, name := range options.Key {
		if hidden[name] {
			return comparisonPlan{}, fmt.Errorf("recordsetcompare: requested key is hidden by policy")
		}
		if keySet[name] {
			return comparisonPlan{}, fmt.Errorf("recordsetcompare: duplicate key column %q", name)
		}
		if !sharedSet[name] {
			return comparisonPlan{}, fmt.Errorf("recordsetcompare: key column %q is missing from the shared columns", name)
		}
		keySet[name] = true
	}
	distributionAt := [2]int{-1, -1}
	if options.DistributionColumn != "" {
		if hidden[options.DistributionColumn] {
			return comparisonPlan{}, fmt.Errorf("recordsetcompare: requested distribution column is hidden by policy")
		}
		if !sharedSet[options.DistributionColumn] {
			return comparisonPlan{}, fmt.Errorf("recordsetcompare: distribution column %q is missing from the shared columns", options.DistributionColumn)
		}
		distributionAt = [2]int{leftColumns[options.DistributionColumn], rightColumns[options.DistributionColumn]}
	}
	return comparisonPlan{
		leftColumns: leftColumns, rightColumns: rightColumns,
		shared: shared, sharedNames: sharedNames,
		keyNames: append([]string(nil), options.Key...), keySet: keySet,
		oneSided: oneSided, leftReceipt: leftReceipt, rightReceipt: rightReceipt,
		distribution: options.DistributionColumn, distributionAt: distributionAt,
		limit: limit, observer: options.Observer,
	}, nil
}

func compareOrderedPrepared(left, right OrderedRecordset, plan comparisonPlan) (apicontract.CompareResult, error) {
	result := apicontract.CompareResult{
		Left: plan.leftReceipt, Right: plan.rightReceipt,
		Columns: plan.shared, Key: append([]string(nil), plan.keyNames...),
		Added: []apicontract.CompareRow{}, Removed: []apicontract.CompareRow{}, Changed: []apicontract.CompareChangedRow{},
		Summary:       apicontract.CompareSummary{ColumnsOnlyOnOneSide: plan.oneSided},
		PolicyLimited: rowsFiltered(plan.leftReceipt.Limitations) || rowsFiltered(plan.rightReceipt.Limitations),
	}
	tracker := newDistributionTracker(plan.distribution)
	rowCounts := [2]int{}
	leftRecords := orderedRowsToRecords("left", left.Rows, left.Columns, plan.leftColumns, plan, 0, &rowCounts[0], tracker)
	rightRecords := orderedRowsToRecords("right", right.Rows, right.Columns, plan.rightColumns, plan, 1, &rowCounts[1], tracker)
	diffs := recordops.DiffFunc(leftRecords, []recordops.RecordSeq[string]{rightRecords}, stringsLess,
		recordops.WithIncludeMatched(), recordops.WithIgnoreFields(plan.keyNames...))

	emitted := 0
	var ordinal uint64
	for diff, err := range diffs {
		if err != nil {
			switch {
			case errors.Is(err, recordops.ErrDuplicateID):
				return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: duplicate key in ordered stream: %w", err)
			case errors.Is(err, recordops.ErrUnsortedInput):
				return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: ordered stream is not strictly ascending by key: %w", err)
			default:
				return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: compare ordered streams: %w", err)
			}
		}
		recordResult, changes, err := classifyDiff(diff, ordinal, plan)
		if err != nil {
			return apicontract.CompareResult{}, err
		}
		if plan.observer != nil {
			if err := plan.observer(recordResult); err != nil {
				return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: observer at ordinal %d: %w", ordinal, err)
			}
		}
		switch recordResult.State {
		case RecordAdded:
			result.Summary.Added++
			if emitted < plan.limit {
				result.Added = append(result.Added, apicontract.CompareRow{Key: cloneValues(recordResult.Key), Row: cloneValues(recordResult.Row)})
				emitted++
			}
		case RecordRemoved:
			result.Summary.Removed++
			if emitted < plan.limit {
				result.Removed = append(result.Removed, apicontract.CompareRow{Key: cloneValues(recordResult.Key), Row: cloneValues(recordResult.Row)})
				emitted++
			}
		case RecordChanged:
			result.Summary.Changed++
			if emitted < plan.limit {
				result.Changed = append(result.Changed, apicontract.CompareChangedRow{Key: cloneValues(recordResult.Key), Columns: changes})
				emitted++
			}
		case RecordMatched:
			result.Summary.Unchanged++
		default:
			return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: unsupported record state %q", recordResult.State)
		}
		ordinal++
	}
	if rowCounts[0] != plan.leftReceipt.RowCount || rowCounts[1] != plan.rightReceipt.RowCount {
		return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: receipt rowCount must match the supplied ordered stream")
	}
	totalDifferences := result.Summary.Added + result.Summary.Removed + result.Summary.Changed
	result.Truncated = emitted < totalDifferences
	if plan.distribution != "" {
		result.Distribution = tracker.result(rowCounts[0], rowCounts[1])
	}
	if err := result.Validate(); err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: invalid result: %w", err)
	}
	return result, nil
}

func orderedRowsToRecords(side string, rows OrderedRows, columns []apicontract.Column, indexes map[string]int, plan comparisonPlan, sideIndex int, rowCount *int, tracker *distributionTracker) recordops.RecordSeq[string] {
	return func(yield func(record.WithID[string], error) bool) {
		var zero record.WithID[string]
		rowIndex := 0
		for row, err := range rows {
			if err != nil {
				yield(zero, fmt.Errorf("%s ordered recordset: %w", side, err))
				return
			}
			if err := validateOrderedRow(row, columns, rowIndex); err != nil {
				yield(zero, fmt.Errorf("%s ordered recordset: %w", side, err))
				return
			}
			key := make([]apicontract.TypedValue, len(plan.keyNames))
			for i, name := range plan.keyNames {
				key[i] = row[indexes[name]]
				if key[i].Type == apicontract.ValueTypeNull {
					yield(zero, fmt.Errorf("%s ordered recordset: row %d has null/missing key column %q", side, rowIndex, name))
					return
				}
			}
			data := make(map[string]any, len(plan.sharedNames))
			for _, name := range plan.sharedNames {
				data[name] = row[indexes[name]]
			}
			if tracker != nil {
				tracker.add(row[plan.distributionAt[sideIndex]], sideIndex == 0)
			}
			*rowCount++
			rowIndex++
			sortKey, err := TypedKeySortKey(key)
			if err != nil {
				yield(zero, fmt.Errorf("%s ordered recordset: row %d key: %w", side, rowIndex-1, err))
				return
			}
			item := record.WithID[string]{ID: sortKey, Record: record.NewRecordWithoutKey(data)}
			if !yield(item, nil) {
				return
			}
		}
	}
}

func validateOrderedRow(row []apicontract.TypedValue, columns []apicontract.Column, rowIndex int) error {
	if len(row) != len(columns) {
		return fmt.Errorf("row %d has %d values, want %d (one per column)", rowIndex, len(row), len(columns))
	}
	for columnIndex, value := range row {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("row %d column %d: %w", rowIndex, columnIndex, err)
		}
		if value.Type != apicontract.ValueTypeNull && string(value.Type) != columns[columnIndex].Type {
			return fmt.Errorf("row %d column %d does not match declared column type", rowIndex, columnIndex)
		}
	}
	return nil
}

func classifyDiff(diff recordops.IDDiff[string], ordinal uint64, plan comparisonPlan) (ComparedRecord, []apicontract.CompareColumnChange, error) {
	if len(diff.Candidates) != 1 {
		return ComparedRecord{}, nil, fmt.Errorf("recordsetcompare: DALgo diff returned %d candidates, want 1", len(diff.Candidates))
	}
	candidate := diff.Candidates[0]
	var (
		row     []apicontract.TypedValue
		changes []apicontract.CompareColumnChange
		deltas  []RecordDelta
		state   RecordState
		err     error
	)
	switch candidate.Status {
	case recordops.Extra:
		state = RecordAdded
		row, err = valuesFromFields(candidate.Fields, plan.sharedNames)
	case recordops.Missing:
		state = RecordRemoved
		if diff.Baseline == nil {
			err = fmt.Errorf("missing right row has no left baseline")
		} else {
			row, err = valuesFromFields(diff.Baseline.Fields, plan.sharedNames)
		}
	case recordops.Matched:
		state = RecordMatched
		if diff.Baseline == nil {
			err = fmt.Errorf("matched row has no left baseline")
		} else {
			row, err = valuesFromFields(diff.Baseline.Fields, plan.sharedNames)
		}
	case recordops.Changed:
		state = RecordChanged
		if diff.Baseline == nil {
			err = fmt.Errorf("changed row has no left baseline")
		} else {
			row, err = valuesFromFields(diff.Baseline.Fields, plan.sharedNames)
			if err == nil {
				changes, deltas, err = changedFields(row, candidate.Fields, plan)
			}
		}
	default:
		err = fmt.Errorf("unknown DALgo record status %d", candidate.Status)
	}
	if err != nil {
		return ComparedRecord{}, nil, fmt.Errorf("recordsetcompare: classify key %q: %w", diff.ID, err)
	}
	key := make([]apicontract.TypedValue, len(plan.keyNames))
	sharedIndex := make(map[string]int, len(plan.sharedNames))
	for i, name := range plan.sharedNames {
		sharedIndex[name] = i
	}
	for i, name := range plan.keyNames {
		key[i] = row[sharedIndex[name]]
	}
	return ComparedRecord{
		Ordinal: ordinal, SortKey: diff.ID, State: state,
		Key: cloneValues(key), Row: cloneValues(row), Deltas: deltas,
	}, changes, nil
}

func valuesFromFields(fields []recordops.FieldValue, names []string) ([]apicontract.TypedValue, error) {
	byName := make(map[string]apicontract.TypedValue, len(fields))
	for _, field := range fields {
		if field.Absent {
			return nil, fmt.Errorf("shared field %q is absent", field.Name)
		}
		value, ok := field.Value.(apicontract.TypedValue)
		if !ok {
			return nil, fmt.Errorf("shared field %q has unexpected value type %T", field.Name, field.Value)
		}
		if _, exists := byName[field.Name]; exists {
			return nil, fmt.Errorf("duplicate shared field %q", field.Name)
		}
		byName[field.Name] = value
	}
	values := make([]apicontract.TypedValue, len(names))
	for i, name := range names {
		value, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("shared field %q is missing", name)
		}
		values[i] = value
	}
	if len(byName) != len(names) {
		return nil, fmt.Errorf("record contains a field outside the shared-visible columns")
	}
	return values, nil
}

func changedFields(baseline []apicontract.TypedValue, fields []recordops.FieldValue, plan comparisonPlan) ([]apicontract.CompareColumnChange, []RecordDelta, error) {
	sharedIndex := make(map[string]int, len(plan.sharedNames))
	for i, name := range plan.sharedNames {
		sharedIndex[name] = i
	}
	changes := make([]apicontract.CompareColumnChange, 0, len(fields))
	deltas := make([]RecordDelta, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		index, ok := sharedIndex[field.Name]
		if !ok {
			return nil, nil, fmt.Errorf("changed field is outside the shared-visible columns")
		}
		if plan.keySet[field.Name] {
			return nil, nil, fmt.Errorf("key field %q was returned as changed", field.Name)
		}
		if seen[field.Name] {
			return nil, nil, fmt.Errorf("duplicate changed field %q", field.Name)
		}
		seen[field.Name] = true
		if field.Absent {
			return nil, nil, fmt.Errorf("shared field %q is absent from right row", field.Name)
		}
		right, ok := field.Value.(apicontract.TypedValue)
		if !ok {
			return nil, nil, fmt.Errorf("changed field %q has unexpected value type %T", field.Name, field.Value)
		}
		changes = append(changes, apicontract.CompareColumnChange{Column: field.Name, Left: baseline[index], Right: right})
		deltas = append(deltas, RecordDelta{Column: field.Name, Value: right})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Column < changes[j].Column })
	sort.Slice(deltas, func(i, j int) bool { return deltas[i].Column < deltas[j].Column })
	return changes, deltas, nil
}

func materializedOrderedRows(recordset apicontract.Recordset, indexes map[string]int, keyNames []string) (OrderedRows, error) {
	type keyedRow struct {
		row     []apicontract.TypedValue
		sortKey string
	}
	rows := make([]keyedRow, len(recordset.Rows))
	for rowIndex, row := range recordset.Rows {
		for _, name := range keyNames {
			if row[indexes[name]].Type == apicontract.ValueTypeNull {
				return nil, fmt.Errorf("row %d has null/missing key column %q", rowIndex, name)
			}
		}
		sortKey, err := TypedKeySortKey(rowKey(row, indexes, keyNames))
		if err != nil {
			return nil, fmt.Errorf("row %d key: %w", rowIndex, err)
		}
		rows[rowIndex] = keyedRow{row: row, sortKey: sortKey}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].sortKey < rows[j].sortKey
	})
	return func(yield func([]apicontract.TypedValue, error) bool) {
		for _, item := range rows {
			if !yield(item.row, nil) {
				return
			}
		}
	}, nil
}

func rowKey(row []apicontract.TypedValue, indexes map[string]int, names []string) []apicontract.TypedValue {
	key := make([]apicontract.TypedValue, len(names))
	for i, name := range names {
		key[i] = row[indexes[name]]
	}
	return key
}

func stringsLess(left, right string) bool { return left < right }

type distributionCounts struct {
	left  int
	right int
	value apicontract.TypedValue
}

// distributionTracker retains only the smallest contract-visible values. Its
// memory is bounded by CompareDistributionMaximumValues even for huge streams.
type distributionTracker struct {
	column    string
	entries   map[string]*distributionCounts
	truncated bool
}

func newDistributionTracker(column string) *distributionTracker {
	if column == "" {
		return nil
	}
	return &distributionTracker{column: column, entries: make(map[string]*distributionCounts, apicontract.CompareDistributionMaximumValues)}
}

func (t *distributionTracker) add(value apicontract.TypedValue, left bool) {
	identity := encodeTuple([]apicontract.TypedValue{value})
	if entry := t.entries[identity]; entry != nil {
		if left {
			entry.left++
		} else {
			entry.right++
		}
		return
	}
	if len(t.entries) == apicontract.CompareDistributionMaximumValues {
		t.truncated = true
		largestKey := ""
		for key, entry := range t.entries {
			if largestKey == "" || compareDistributionValue(t.entries[largestKey].value, largestKey, entry.value, key) < 0 {
				largestKey = key
			}
		}
		if compareDistributionValue(value, identity, t.entries[largestKey].value, largestKey) >= 0 {
			return
		}
		delete(t.entries, largestKey)
	}
	entry := &distributionCounts{value: value}
	if left {
		entry.left = 1
	} else {
		entry.right = 1
	}
	t.entries[identity] = entry
}

func (t *distributionTracker) result(leftTotal, rightTotal int) *apicontract.CompareDistribution {
	keys := make([]string, 0, len(t.entries))
	for key := range t.entries {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return compareDistributionValue(t.entries[keys[i]].value, keys[i], t.entries[keys[j]].value, keys[j]) < 0
	})
	result := &apicontract.CompareDistribution{Column: t.column, Values: []apicontract.CompareDistributionValue{}, Truncated: t.truncated}
	for _, key := range keys {
		entry := t.entries[key]
		leftPct, rightPct := percentage(entry.left, leftTotal), percentage(entry.right, rightTotal)
		var ratio *float64
		if rightPct != 0 {
			value := leftPct / rightPct
			ratio = &value
		}
		result.Values = append(result.Values, apicontract.CompareDistributionValue{
			Value: entry.value,
			Left:  apicontract.CompareDistributionSide{Count: entry.left, Pct: leftPct},
			Right: apicontract.CompareDistributionSide{Count: entry.right, Pct: rightPct}, Ratio: ratio,
		})
	}
	return result
}

func compareDistributionValue(left apicontract.TypedValue, leftIdentity string, right apicontract.TypedValue, rightIdentity string) int {
	leftKey, leftErr := apicontract.TypedValueSortKey(left)
	rightKey, rightErr := apicontract.TypedValueSortKey(right)
	if leftErr == nil && rightErr == nil {
		if order := strings.Compare(leftKey, rightKey); order != 0 {
			return order
		}
	}
	if order := strings.Compare(leftIdentity, rightIdentity); order != 0 {
		return order
	}
	return 0
}
