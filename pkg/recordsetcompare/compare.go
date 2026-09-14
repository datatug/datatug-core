// Package recordsetcompare provides a deterministic, pure comparison of two
// policy-filtered recordsets. It has no dependency on the schema comparator.
package recordsetcompare

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

type Options struct {
	Key                []string
	DistributionColumn string
	Limit              int
}

type indexedRow struct {
	key    []apicontract.TypedValue
	values []apicontract.TypedValue
}

type distributionCounts struct {
	left  int
	right int
	value apicontract.TypedValue
}

func Compare(left, right apicontract.Recordset, leftReceipt, rightReceipt apicontract.CompareSideReceipt, options Options) (apicontract.CompareResult, error) {
	if err := leftReceipt.Validate(); err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("left receipt: %w", err)
	}
	if err := rightReceipt.Validate(); err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("right receipt: %w", err)
	}
	if err := left.Validate(); err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("left recordset: %w", err)
	}
	if err := right.Validate(); err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("right recordset: %w", err)
	}
	if err := validateRecordsetColumnTypes(left); err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("left recordset: %w", err)
	}
	if err := validateRecordsetColumnTypes(right); err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("right recordset: %w", err)
	}
	if leftReceipt.RowCount != len(left.Rows) || rightReceipt.RowCount != len(right.Rows) {
		return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: receipt rowCount must match the supplied recordset")
	}
	hidden := hiddenColumns(leftReceipt.Limitations, rightReceipt.Limitations)
	if len(options.Key) == 0 {
		return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: key is required")
	}
	limit := options.Limit
	if limit == 0 {
		limit = apicontract.CompareDefaultLimit
	}
	if limit < 1 || limit > apicontract.CompareMaximumLimit {
		return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: limit must be between 1 and %d", apicontract.CompareMaximumLimit)
	}

	leftColumns, err := columnsByName(left.Columns, hidden)
	if err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("left recordset: %w", err)
	}
	rightColumns, err := columnsByName(right.Columns, hidden)
	if err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("right recordset: %w", err)
	}
	shared, oneSided, err := intersectColumns(left.Columns, right.Columns, leftColumns, rightColumns, hidden)
	if err != nil {
		return apicontract.CompareResult{}, err
	}
	sharedNames := make([]string, len(shared))
	for i := range shared {
		sharedNames[i] = shared[i].Name
	}
	sharedSet := make(map[string]bool, len(sharedNames))
	for _, name := range sharedNames {
		sharedSet[name] = true
	}
	keySet := map[string]bool{}
	for _, name := range options.Key {
		if hidden[name] {
			return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: requested key is hidden by policy")
		}
		if keySet[name] {
			return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: duplicate key column %q", name)
		}
		if !sharedSet[name] {
			return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: key column %q is missing from the shared columns", name)
		}
		keySet[name] = true
	}
	if options.DistributionColumn != "" {
		if hidden[options.DistributionColumn] {
			return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: requested distribution column is hidden by policy")
		}
		if !sharedSet[options.DistributionColumn] {
			return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: distribution column %q is missing from the shared columns", options.DistributionColumn)
		}
	}

	leftRows, err := indexRows(left, leftColumns, sharedNames, options.Key)
	if err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("left recordset: %w", err)
	}
	rightRows, err := indexRows(right, rightColumns, sharedNames, options.Key)
	if err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("right recordset: %w", err)
	}
	keys := make([]string, 0, len(leftRows)+len(rightRows))
	for key := range leftRows {
		keys = append(keys, key)
	}
	for key := range rightRows {
		if _, ok := leftRows[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		leftKey := leftRows[keys[i]].key
		if len(leftKey) == 0 {
			leftKey = rightRows[keys[i]].key
		}
		rightKey := leftRows[keys[j]].key
		if len(rightKey) == 0 {
			rightKey = rightRows[keys[j]].key
		}
		return compareTuple(leftKey, rightKey) < 0
	})

	result := apicontract.CompareResult{
		Left: leftReceipt, Right: rightReceipt, Columns: shared, Key: append([]string(nil), options.Key...),
		Added: []apicontract.CompareRow{}, Removed: []apicontract.CompareRow{}, Changed: []apicontract.CompareChangedRow{},
		Summary:       apicontract.CompareSummary{ColumnsOnlyOnOneSide: oneSided},
		PolicyLimited: rowsFiltered(leftReceipt.Limitations) || rowsFiltered(rightReceipt.Limitations),
	}
	emitted := 0
	for _, encoded := range keys {
		leftRow, inLeft := leftRows[encoded]
		rightRow, inRight := rightRows[encoded]
		switch {
		case !inLeft:
			result.Summary.Added++
			if emitted < limit {
				result.Added = append(result.Added, apicontract.CompareRow{Key: cloneValues(rightRow.key), Row: cloneValues(rightRow.values)})
				emitted++
			}
		case !inRight:
			result.Summary.Removed++
			if emitted < limit {
				result.Removed = append(result.Removed, apicontract.CompareRow{Key: cloneValues(leftRow.key), Row: cloneValues(leftRow.values)})
				emitted++
			}
		default:
			changes := make([]apicontract.CompareColumnChange, 0)
			for index, column := range shared {
				if keySet[column.Name] || typedEqual(leftRow.values[index], rightRow.values[index]) {
					continue
				}
				changes = append(changes, apicontract.CompareColumnChange{Column: column.Name, Left: leftRow.values[index], Right: rightRow.values[index]})
			}
			if len(changes) == 0 {
				result.Summary.Unchanged++
				continue
			}
			result.Summary.Changed++
			if emitted < limit {
				result.Changed = append(result.Changed, apicontract.CompareChangedRow{Key: cloneValues(leftRow.key), Columns: changes})
				emitted++
			}
		}
	}
	totalDifferences := result.Summary.Added + result.Summary.Removed + result.Summary.Changed
	result.Truncated = emitted < totalDifferences
	if options.DistributionColumn != "" {
		result.Distribution = distribution(left, right, leftColumns[options.DistributionColumn], rightColumns[options.DistributionColumn], options.DistributionColumn)
	}
	if err := result.Validate(); err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("recordsetcompare: invalid result: %w", err)
	}
	return result, nil
}

func columnsByName(columns []apicontract.Column, hidden map[string]bool) (map[string]int, error) {
	result := make(map[string]int, len(columns))
	for index, column := range columns {
		if hidden[column.Name] {
			continue
		}
		if prior, ok := result[column.Name]; ok {
			return nil, fmt.Errorf("duplicate column %q at indexes %d and %d", column.Name, prior, index)
		}
		result[column.Name] = index
	}
	return result, nil
}

func validateRecordsetColumnTypes(recordset apicontract.Recordset) error {
	for rowIndex, row := range recordset.Rows {
		for columnIndex, value := range row {
			if value.Type != apicontract.ValueTypeNull && string(value.Type) != recordset.Columns[columnIndex].Type {
				return fmt.Errorf("row %d column %d does not match declared column type", rowIndex, columnIndex)
			}
		}
	}
	return nil
}

func intersectColumns(leftList, rightList []apicontract.Column, left, right map[string]int, hidden map[string]bool) ([]apicontract.Column, []apicontract.CompareOneSidedColumn, error) {
	sharedNames := make([]string, 0)
	oneSided := make([]apicontract.CompareOneSidedColumn, 0)
	for name := range left {
		if hidden[name] {
			continue
		}
		if _, ok := right[name]; ok {
			sharedNames = append(sharedNames, name)
		} else {
			oneSided = append(oneSided, apicontract.CompareOneSidedColumn{Column: name, Side: apicontract.CompareColumnLeft})
		}
	}
	for name := range right {
		if hidden[name] {
			continue
		}
		if _, ok := left[name]; !ok {
			oneSided = append(oneSided, apicontract.CompareOneSidedColumn{Column: name, Side: apicontract.CompareColumnRight})
		}
	}
	sort.Strings(sharedNames)
	sort.Slice(oneSided, func(i, j int) bool {
		if oneSided[i].Column == oneSided[j].Column {
			return oneSided[i].Side < oneSided[j].Side
		}
		return oneSided[i].Column < oneSided[j].Column
	})
	shared := make([]apicontract.Column, 0, len(sharedNames))
	for _, name := range sharedNames {
		leftColumn, rightColumn := left[name], right[name]
		if leftList[leftColumn].Type != rightList[rightColumn].Type {
			return nil, nil, fmt.Errorf("recordsetcompare: shared column %q has incompatible types %q and %q", name, leftList[leftColumn].Type, rightList[rightColumn].Type)
		}
		shared = append(shared, leftList[leftColumn])
	}
	return shared, oneSided, nil
}

func indexRows(recordset apicontract.Recordset, indexes map[string]int, sharedNames, keyNames []string) (map[string]indexedRow, error) {
	result := make(map[string]indexedRow, len(recordset.Rows))
	for rowIndex, row := range recordset.Rows {
		key := make([]apicontract.TypedValue, len(keyNames))
		for i, name := range keyNames {
			key[i] = row[indexes[name]]
			if key[i].Type == apicontract.ValueTypeNull {
				return nil, fmt.Errorf("row %d has null/missing key column %q", rowIndex, name)
			}
		}
		encoded := encodeTuple(key)
		if _, exists := result[encoded]; exists {
			return nil, fmt.Errorf("duplicate key at row %d", rowIndex)
		}
		values := make([]apicontract.TypedValue, len(sharedNames))
		for i, name := range sharedNames {
			values[i] = row[indexes[name]]
		}
		result[encoded] = indexedRow{key: key, values: values}
	}
	return result, nil
}

func typedEqual(left, right apicontract.TypedValue) bool { return left == right }

func encodeTuple(values []apicontract.TypedValue) string {
	var b strings.Builder
	for _, value := range values {
		if value.Type == apicontract.ValueTypeNumber && value.Num == 0 {
			value.Num = 0
		}
		encoded, _ := json.Marshal(value)
		b.WriteString(strconv.Itoa(len(encoded)))
		b.WriteByte(':')
		b.Write(encoded)
	}
	return b.String()
}

func compareTuple(left, right []apicontract.TypedValue) int {
	for i := 0; i < len(left) && i < len(right); i++ {
		if order := strings.Compare(typedSortKey(left[i]), typedSortKey(right[i])); order != 0 {
			return order
		}
	}
	return len(left) - len(right)
}

func typedSortKey(value apicontract.TypedValue) string {
	switch value.Type {
	case apicontract.ValueTypeNumber:
		if value.Num == 0 {
			return string(value.Type) + ":0"
		}
		return string(value.Type) + ":" + strconv.FormatFloat(value.Num, 'g', -1, 64)
	case apicontract.ValueTypeBoolean:
		return string(value.Type) + ":" + strconv.FormatBool(value.Bool)
	default:
		return string(value.Type) + ":" + value.Str
	}
}

func distribution(left, right apicontract.Recordset, leftIndex, rightIndex int, column string) *apicontract.CompareDistribution {
	all := map[string]*distributionCounts{}
	for _, row := range left.Rows {
		addDistribution(all, row[leftIndex], true)
	}
	for _, row := range right.Rows {
		addDistribution(all, row[rightIndex], false)
	}
	keys := make([]string, 0, len(all))
	for key := range all {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.Compare(typedSortKey(all[keys[i]].value), typedSortKey(all[keys[j]].value)) < 0
	})
	truncated := len(keys) > apicontract.CompareDistributionMaximumValues
	if truncated {
		keys = keys[:apicontract.CompareDistributionMaximumValues]
	}
	result := &apicontract.CompareDistribution{Column: column, Values: []apicontract.CompareDistributionValue{}, Truncated: truncated}
	for _, key := range keys {
		entry := all[key]
		leftPct, rightPct := percentage(entry.left, len(left.Rows)), percentage(entry.right, len(right.Rows))
		var ratio *float64
		if rightPct != 0 {
			value := leftPct / rightPct
			ratio = &value
		}
		result.Values = append(result.Values, apicontract.CompareDistributionValue{Value: entry.value, Left: apicontract.CompareDistributionSide{Count: entry.left, Pct: leftPct}, Right: apicontract.CompareDistributionSide{Count: entry.right, Pct: rightPct}, Ratio: ratio})
	}
	return result
}

func addDistribution(all map[string]*distributionCounts, value apicontract.TypedValue, left bool) {
	key := encodeTuple([]apicontract.TypedValue{value})
	entry := all[key]
	if entry == nil {
		entry = &distributionCounts{value: value}
		all[key] = entry
	}
	if left {
		entry.left++
	} else {
		entry.right++
	}
}

func percentage(count, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(count) * 100 / float64(total)
}
func rowsFiltered(limitations []apicontract.Limitation) bool {
	for _, limitation := range limitations {
		if limitation.RowsFiltered {
			return true
		}
	}
	return false
}

func hiddenColumns(groups ...[]apicontract.Limitation) map[string]bool {
	hidden := map[string]bool{}
	for _, limitations := range groups {
		for _, limitation := range limitations {
			for _, column := range limitation.HiddenColumns {
				hidden[column] = true
			}
		}
	}
	return hidden
}

func cloneValues(values []apicontract.TypedValue) []apicontract.TypedValue {
	return append([]apicontract.TypedValue(nil), values...)
}
