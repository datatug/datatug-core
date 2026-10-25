package challenge

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

// Row ordering modes. Unordered comparison is the default contract for result
// sets; ordered comparison is asserted only when the manifest declares it.
const (
	OrderUnordered = "unordered"
	OrderOrdered   = "ordered"
)

// Stable comparison problem codes. They are wire values used in feedback.
const (
	ProblemColumnCount = "column-count"
	ProblemColumnName  = "column-name"
	// ProblemColumnType is reserved. Column metadata types describe a column,
	// not a graded cell; SQLite storage class is compared per typed cell.
	ProblemColumnType     = "column-type"
	ProblemRowShape       = "row-shape"
	ProblemRowValueType   = "row-value-type"
	ProblemRowOrder       = "row-order"
	ProblemRowContent     = "row-content"
	ProblemInvalidOptions = "invalid-options"
	ProblemInvariant      = "invariant"
)

// RowOptions controls typed row comparison.
type RowOptions struct {
	Ordering           string `json:"ordering"`
	RequireColumnNames bool   `json:"requireColumnNames,omitempty"`
	NumericTolerance   string `json:"numericTolerance,omitempty"`
}

// RowDiff is a bounded, deterministic description of why two typed row sets
// differ. Missing and Unexpected count whole rows in an unordered comparison;
// ordered comparisons report the first differing position instead.
type RowDiff struct {
	Match      bool     `json:"match"`
	Problem    string   `json:"problem,omitempty"`
	Message    string   `json:"message,omitempty"`
	Missing    int      `json:"missing,omitempty"`
	Unexpected int      `json:"unexpected,omitempty"`
	Examples   []string `json:"examples,omitempty"`
}

// Feedback examples are bounded so a hostile cell cannot inflate an answer or
// its wire payload. The limit counts Unicode code points, not UTF-16 units, so
// truncation never splits a surrogate pair or an astral character.
const (
	maxFeedbackExamples     = 3
	maxFeedbackExampleRunes = 240
	feedbackEllipsis        = "…"
)

// CompareTypedRows compares a declared expected recordset with an observed
// one. Column metadata is validated for supported shape, but a column's declared
// type is descriptive and is not compared with the observed metadata or the
// cells in it: SQLite storage class participates in equality per typed cell.
// Values are matched by declared column position; column count is exact; column
// aliases are ignored unless RequireColumnNames is set. Storage classes remain
// distinct (INTEGER 2 differs from REAL 2.0) unless an explicit non-negative
// numeric tolerance is declared.
func CompareTypedRows(expectedColumns, actualColumns []apicontract.Column, expectedRows, actualRows [][]apicontract.TypedValue, options RowOptions) RowDiff {
	if options.Ordering != OrderUnordered && options.Ordering != OrderOrdered {
		return RowDiff{Problem: ProblemInvalidOptions, Message: fmt.Sprintf("ordering must be %q or %q", OrderUnordered, OrderOrdered)}
	}
	tolerance, err := parseTolerance(options.NumericTolerance)
	if err != nil {
		return RowDiff{Problem: ProblemInvalidOptions, Message: err.Error()}
	}
	if diff := validateColumnShape(expectedColumns); diff != nil {
		return *diff
	}
	if diff := validateObservedColumns(actualColumns); diff != nil {
		return *diff
	}
	if len(expectedColumns) != len(actualColumns) {
		return RowDiff{Problem: ProblemColumnCount, Message: fmt.Sprintf("expected %d columns, observed %d", len(expectedColumns), len(actualColumns))}
	}
	for index := range expectedColumns {
		if options.RequireColumnNames && expectedColumns[index].Name != actualColumns[index].Name {
			return RowDiff{Problem: ProblemColumnName, Message: fmt.Sprintf("column %d expected name %q, observed %q", index+1, expectedColumns[index].Name, actualColumns[index].Name)}
		}
	}
	if diff := validateRowTypes(expectedColumns, expectedRows); diff != nil {
		return *diff
	}
	if diff := validateRowTypes(actualColumns, actualRows); diff != nil {
		return *diff
	}

	if options.Ordering == OrderOrdered {
		if len(expectedRows) != len(actualRows) {
			return RowDiff{Problem: ProblemRowContent, Message: fmt.Sprintf("expected %d rows, observed %d", len(expectedRows), len(actualRows))}
		}
		for rowIndex := range expectedRows {
			for columnIndex := range expectedRows[rowIndex] {
				if !typedValueEqual(expectedRows[rowIndex][columnIndex], actualRows[rowIndex][columnIndex], tolerance) {
					return RowDiff{
						Problem: ProblemRowOrder,
						Message: fmt.Sprintf("row %d column %d differs: expected %s, observed %s",
							rowIndex+1, columnIndex+1, renderValue(expectedRows[rowIndex][columnIndex]), renderValue(actualRows[rowIndex][columnIndex])),
					}
				}
			}
		}
		return RowDiff{Match: true}
	}

	matchedExpected, matchedActual := matchRows(expectedRows, actualRows, tolerance)
	var missing, unexpected int
	var examples []string
	for index, expected := range expectedRows {
		if !matchedExpected[index] {
			missing++
			if len(examples) < maxFeedbackExamples {
				examples = append(examples, "missing: "+renderRow(expected))
			}
		}
	}
	for index, actual := range actualRows {
		if !matchedActual[index] {
			unexpected++
			if len(examples) < maxFeedbackExamples {
				examples = append(examples, "unexpected: "+renderRow(actual))
			}
		}
	}
	if missing == 0 && unexpected == 0 {
		return RowDiff{Match: true}
	}
	return RowDiff{
		Problem:    ProblemRowContent,
		Message:    fmt.Sprintf("%d expected row(s) missing, %d unexpected row(s); duplicate multiplicity and NULL are significant", missing, unexpected),
		Missing:    missing,
		Unexpected: unexpected,
		Examples:   boundedExamples(examples),
	}
}

// boundedExamples returns a new, deterministically ordered slice capped at
// maxFeedbackExamples entries, each valid UTF-8 and capped at
// maxFeedbackExampleRunes code points. Truncation appends an explicit ellipsis
// marker, so a clipped example can never be mistaken for a complete one.
func boundedExamples(examples []string) []string {
	if len(examples) == 0 {
		return nil
	}
	limit := len(examples)
	if limit > maxFeedbackExamples {
		limit = maxFeedbackExamples
	}
	bounded := make([]string, 0, limit)
	for _, example := range examples[:limit] {
		bounded = append(bounded, boundedExample(example))
	}
	return bounded
}

func boundedExample(example string) string {
	valid := strings.ToValidUTF8(example, "\uFFFD")
	if utf8.RuneCountInString(valid) <= maxFeedbackExampleRunes {
		return valid
	}
	runes := []rune(valid)
	return string(runes[:maxFeedbackExampleRunes]) + feedbackEllipsis
}

// matchRows returns a deterministic maximum matching. Exact typed equality is
// transitive, so a simple first-fit match is already optimal. A declared
// numeric tolerance is not transitive, so a greedy first fit can reject a
// valid pairing; use augmenting paths over the small, bounded row sets.
func matchRows(expectedRows, actualRows [][]apicontract.TypedValue, tolerance *big.Rat) ([]bool, []bool) {
	matchedExpected := make([]bool, len(expectedRows))
	matchedActual := make([]bool, len(actualRows))
	if tolerance == nil {
		for expectedIndex, expected := range expectedRows {
			for actualIndex, actual := range actualRows {
				if !matchedActual[actualIndex] && typedRowsEqual(expected, actual, tolerance) {
					matchedExpected[expectedIndex] = true
					matchedActual[actualIndex] = true
					break
				}
			}
		}
		return matchedExpected, matchedActual
	}

	adjacency := make([][]int, len(expectedRows))
	for expectedIndex, expected := range expectedRows {
		for actualIndex, actual := range actualRows {
			if typedRowsEqual(expected, actual, tolerance) {
				adjacency[expectedIndex] = append(adjacency[expectedIndex], actualIndex)
			}
		}
	}
	owner := make([]int, len(actualRows))
	for index := range owner {
		owner[index] = -1
	}
	var assign func(expectedIndex int, seen []bool) bool
	assign = func(expectedIndex int, seen []bool) bool {
		for _, actualIndex := range adjacency[expectedIndex] {
			if seen[actualIndex] {
				continue
			}
			seen[actualIndex] = true
			if owner[actualIndex] == -1 || assign(owner[actualIndex], seen) {
				owner[actualIndex] = expectedIndex
				return true
			}
		}
		return false
	}
	for expectedIndex := range expectedRows {
		if assign(expectedIndex, make([]bool, len(actualRows))) {
			matchedExpected[expectedIndex] = true
		}
	}
	for actualIndex, ownerIndex := range owner {
		if ownerIndex >= 0 {
			matchedActual[actualIndex] = true
		}
	}
	return matchedExpected, matchedActual
}

func validateColumnShape(columns []apicontract.Column) *RowDiff {
	if len(columns) == 0 || len(columns) > MaxResultColumns {
		return &RowDiff{Problem: ProblemInvalidOptions, Message: fmt.Sprintf("column count must be between 1 and %d", MaxResultColumns)}
	}
	seen := map[string]bool{}
	for _, column := range columns {
		if err := column.Validate(); err != nil {
			return &RowDiff{Problem: ProblemInvalidOptions, Message: err.Error()}
		}
		if seen[column.Name] {
			return &RowDiff{Problem: ProblemInvalidOptions, Message: fmt.Sprintf("duplicate expected column name %q", column.Name)}
		}
		seen[column.Name] = true
	}
	return nil
}

// validateObservedColumns accepts duplicate observed aliases: they are ignored
// unless the oracle explicitly requires column names.
func validateObservedColumns(columns []apicontract.Column) *RowDiff {
	if len(columns) == 0 || len(columns) > MaxResultColumns {
		return &RowDiff{Problem: ProblemInvalidOptions, Message: fmt.Sprintf("column count must be between 1 and %d", MaxResultColumns)}
	}
	for _, column := range columns {
		if err := column.Validate(); err != nil {
			return &RowDiff{Problem: ProblemInvalidOptions, Message: err.Error()}
		}
	}
	return nil
}

// validateRowTypes checks only challenge-local row shape and typed-cell
// validity. A cell's SQLite storage class is carried by the TypedValue, so it
// need not (and must not) match the descriptive column metadata type.
func validateRowTypes(columns []apicontract.Column, rows [][]apicontract.TypedValue) *RowDiff {
	if len(rows) > MaxResultRows {
		return &RowDiff{Problem: ProblemInvalidOptions, Message: fmt.Sprintf("row count exceeds %d", MaxResultRows)}
	}
	for rowIndex, values := range rows {
		if len(values) != len(columns) {
			return &RowDiff{Problem: ProblemRowShape, Message: fmt.Sprintf("row %d has %d values, expected %d", rowIndex+1, len(values), len(columns))}
		}
		for columnIndex, value := range values {
			if err := value.Validate(); err != nil {
				return &RowDiff{Problem: ProblemRowValueType, Message: fmt.Sprintf("row %d column %d: %s", rowIndex+1, columnIndex+1, err)}
			}
		}
	}
	return nil
}

func typedRowsEqual(expected, actual []apicontract.TypedValue, tolerance *big.Rat) bool {
	if len(expected) != len(actual) {
		return false
	}
	for index := range expected {
		if !typedValueEqual(expected[index], actual[index], tolerance) {
			return false
		}
	}
	return true
}

func typedValueEqual(expected, actual apicontract.TypedValue, tolerance *big.Rat) bool {
	if expected.Type == apicontract.ValueTypeNull || actual.Type == apicontract.ValueTypeNull {
		return expected.Type == apicontract.ValueTypeNull && actual.Type == apicontract.ValueTypeNull
	}
	if expected.Type != actual.Type {
		if tolerance == nil || !isNumericType(string(expected.Type)) || !isNumericType(string(actual.Type)) {
			return false
		}
		return numericValuesWithin(expected, actual, tolerance)
	}
	switch expected.Type {
	case apicontract.ValueTypeNumber:
		if tolerance != nil {
			return numericValuesWithin(expected, actual, tolerance)
		}
		return expected.Num == actual.Num
	case apicontract.ValueTypeInteger, apicontract.ValueTypeDecimal:
		if tolerance != nil {
			return numericValuesWithin(expected, actual, tolerance)
		}
		return expected.Str == actual.Str
	case apicontract.ValueTypeBoolean:
		return expected.Bool == actual.Bool
	default:
		return expected.Str == actual.Str
	}
}

func numericValuesWithin(left, right apicontract.TypedValue, tolerance *big.Rat) bool {
	leftValue, okLeft := numericRat(left)
	rightValue, okRight := numericRat(right)
	if !okLeft || !okRight {
		return false
	}
	difference := new(big.Rat).Sub(leftValue, rightValue)
	difference.Abs(difference)
	return difference.Cmp(tolerance) <= 0
}

func numericRat(value apicontract.TypedValue) (*big.Rat, bool) {
	switch value.Type {
	case apicontract.ValueTypeInteger, apicontract.ValueTypeDecimal:
		parsed, ok := new(big.Rat).SetString(value.Str)
		return parsed, ok
	case apicontract.ValueTypeNumber:
		parsed := new(big.Rat).SetFloat64(value.Num)
		return parsed, parsed != nil
	default:
		return nil, false
	}
}

func isNumericType(typ string) bool {
	return typ == string(apicontract.ValueTypeInteger) || typ == string(apicontract.ValueTypeDecimal) || typ == string(apicontract.ValueTypeNumber)
}

func parseTolerance(value string) (*big.Rat, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > 32 {
		return nil, fmt.Errorf("numeric tolerance is too long")
	}
	if err := apicontract.NewDecimalValue(value).Validate(); err != nil {
		return nil, fmt.Errorf("numeric tolerance must be a canonical non-negative decimal: %w", err)
	}
	parsed, ok := new(big.Rat).SetString(value)
	if !ok || parsed.Sign() < 0 {
		return nil, fmt.Errorf("numeric tolerance must be a canonical non-negative decimal")
	}
	// An explicit "0" is a declared tolerance, not an omitted one: it keeps
	// numeric comparison (and therefore cross-storage-class equality) enabled
	// while allowing no difference. Only an absent value yields nil.
	return parsed, nil
}

func renderRow(row []apicontract.TypedValue) string {
	data, err := json.Marshal(row)
	if err != nil {
		return "<unrenderable>"
	}
	return string(data)
}

func renderValue(value apicontract.TypedValue) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "<unrenderable>"
	}
	return string(data)
}
