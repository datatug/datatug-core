package recordsetcompare

import "github.com/datatug/datatug-core/pkg/apicontract"

// CompareTypedKeys exposes the apicontract portable natural key order from the
// comparison package for ordered-row producers.
func CompareTypedKeys(left, right []apicontract.TypedValue) (int, error) {
	return apicontract.CompareTypedKeys(left, right)
}

// TypedKeySortKey exposes the canonical binary-sortable key encoding used by
// CompareOrdered and comparison-cache pagination.
func TypedKeySortKey(values []apicontract.TypedValue) (string, error) {
	return apicontract.TypedKeySortKey(values)
}
