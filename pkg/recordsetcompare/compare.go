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
	Observer           RecordObserver
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
	prepared, err := prepareComparison(left.Columns, right.Columns, leftReceipt, rightReceipt, options)
	if err != nil {
		return apicontract.CompareResult{}, err
	}
	leftRows, err := materializedOrderedRows(left, prepared.leftColumns, options.Key)
	if err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("left recordset: %w", err)
	}
	rightRows, err := materializedOrderedRows(right, prepared.rightColumns, options.Key)
	if err != nil {
		return apicontract.CompareResult{}, fmt.Errorf("right recordset: %w", err)
	}
	return compareOrderedPrepared(
		OrderedRecordset{Columns: left.Columns, Rows: leftRows},
		OrderedRecordset{Columns: right.Columns, Rows: rightRows},
		prepared,
	)
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
