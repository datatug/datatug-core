package recordsetcompare

import (
	"fmt"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

// OrderRecordset validates every row of a materialized recordset and returns
// it as an OrderedRows stream sorted strictly by key, ready for CompareOrdered
// or an N-way reconcile.Source. It holds the recordset in memory; prefer an
// ordered query for large datasets. Duplicate keys are left in place for the
// consumer's merge to reject.
func OrderRecordset(recordset apicontract.Recordset, key []string) (OrderedRows, error) {
	if len(key) == 0 {
		return nil, fmt.Errorf("recordsetcompare: key is required")
	}
	indexes, err := columnsByName(recordset.Columns, nil)
	if err != nil {
		return nil, fmt.Errorf("recordsetcompare: %w", err)
	}
	for _, name := range key {
		if _, ok := indexes[name]; !ok {
			return nil, fmt.Errorf("recordsetcompare: key column %q is missing", name)
		}
	}
	for rowIndex, row := range recordset.Rows {
		if err := validateOrderedRow(row, recordset.Columns, rowIndex); err != nil {
			return nil, fmt.Errorf("recordsetcompare: %w", err)
		}
	}
	return materializedOrderedRows(recordset, indexes, key)
}

// ValidateOrderedRow checks that a streamed row has one valid value per
// column and that each non-null value matches its column's declared type.
func ValidateOrderedRow(row []apicontract.TypedValue, columns []apicontract.Column, rowIndex int) error {
	return validateOrderedRow(row, columns, rowIndex)
}
