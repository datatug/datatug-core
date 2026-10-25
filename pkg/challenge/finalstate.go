package challenge

import (
	"fmt"
	"math/big"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

// TableResult reports one declared table's exact final-state comparison.
type TableResult struct {
	Table   string `json:"table"`
	Pass    bool   `json:"pass"`
	Problem string `json:"problem,omitempty"`
	Message string `json:"message,omitempty"`
}

// InvariantResult reports one named retention invariant.
type InvariantResult struct {
	ID      string `json:"id"`
	Pass    bool   `json:"pass"`
	Problem string `json:"problem,omitempty"`
	Message string `json:"message,omitempty"`
}

// FinalStateResult is the deterministic outcome of a write answer. Every
// declared table and every invariant must pass.
type FinalStateResult struct {
	Pass       bool              `json:"pass"`
	Problem    string            `json:"problem,omitempty"`
	Message    string            `json:"message,omitempty"`
	Tables     []TableResult     `json:"tables"`
	Invariants []InvariantResult `json:"invariants,omitempty"`
}

// ViolatedInvariant returns the first failed invariant ID, if any.
func (r FinalStateResult) ViolatedInvariant() string {
	for _, invariant := range r.Invariants {
		if !invariant.Pass {
			return invariant.ID
		}
	}
	return ""
}

// EvaluateFinalState compares observed tables with the declared expected
// contents and evaluates each named invariant. It never guesses an expected
// value: the manifest's expected table is the single answer key.
func EvaluateFinalState(oracle FinalStateOracle, actual []TableState) FinalStateResult {
	result := FinalStateResult{Pass: true}
	observed := make(map[string]TableState, len(actual))
	for _, table := range actual {
		if _, exists := observed[table.Name]; exists {
			result.Tables = append(result.Tables, TableResult{Table: table.Name, Problem: ProblemRowContent, Message: "duplicate observed table"})
			result.Pass = false
			continue
		}
		observed[table.Name] = table
	}

	expectedByTable := make(map[string]TableExpectation, len(oracle.Tables))
	for _, expected := range oracle.Tables {
		expectedByTable[expected.Name] = expected
		table, ok := observed[expected.Name]
		if !ok {
			result.Tables = append(result.Tables, TableResult{Table: expected.Name, Problem: ProblemRowContent, Message: "expected final-state table is unavailable"})
			result.Pass = false
			continue
		}
		diff := CompareTypedRows(expected.Columns, table.Columns, expected.Rows, table.Rows, RowOptions{
			Ordering:           OrderUnordered,
			RequireColumnNames: true,
		})
		tableResult := TableResult{Table: expected.Name, Pass: diff.Match, Problem: diff.Problem, Message: diff.Message}
		result.Tables = append(result.Tables, tableResult)
		if !diff.Match {
			result.Pass = false
			if result.Problem == "" {
				result.Problem = diff.Problem
				result.Message = fmt.Sprintf("table %s: %s", expected.Name, diff.Message)
			}
		}
	}
	for _, table := range actual {
		if _, expected := expectedByTable[table.Name]; !expected {
			result.Tables = append(result.Tables, TableResult{Table: table.Name, Problem: ProblemRowContent, Message: "observed table is not declared by the final-state oracle"})
			result.Pass = false
			if result.Problem == "" {
				result.Problem = ProblemRowContent
				result.Message = fmt.Sprintf("unexpected observed table %q", table.Name)
			}
		}
	}

	for _, invariant := range oracle.Invariants {
		invariantResult := evaluateInvariant(invariant, expectedByTable[invariant.Table], observed[invariant.Table])
		result.Invariants = append(result.Invariants, invariantResult)
		if !invariantResult.Pass {
			result.Pass = false
			switch {
			case invariantResult.Problem == ProblemInvariant:
				if result.Problem != ProblemInvariant {
					result.Problem = ProblemInvariant
					result.Message = invariantResult.Message
				}
			case result.Problem == "":
				// A malformed observation keeps its structural problem rather
				// than being reported as a violated retention rule.
				result.Problem = invariantResult.Problem
				result.Message = invariantResult.Message
			}
		}
	}
	return result
}

func evaluateInvariant(invariant Invariant, expected TableExpectation, actual TableState) InvariantResult {
	fail := func(problem, format string, args ...any) InvariantResult {
		return InvariantResult{ID: invariant.ID, Problem: problem, Message: fmt.Sprintf("%s: ", invariant.ID) + fmt.Sprintf(format, args...)}
	}
	if actual.Name == "" {
		return fail(ProblemInvariant, "table %q is unavailable; expected the lowest %s per %v", invariant.Table, invariant.Column, invariant.GroupBy)
	}
	// A row narrower or wider than its declared columns cannot be indexed by
	// position. Stop before grouping or retention lookup so a malformed
	// observation is a recoverable failure, never a panic or a silent pass.
	if diff := validateRowTypes(expected.Columns, expected.Rows); diff != nil {
		return fail(diff.Problem, "expected table %q is malformed: %s", invariant.Table, diff.Message)
	}
	if diff := validateRowTypes(actual.Columns, actual.Rows); diff != nil {
		return fail(diff.Problem, "observed table %q is malformed: %s", invariant.Table, diff.Message)
	}
	expectedIndex := map[string]int{}
	for index, column := range expected.Columns {
		expectedIndex[column.Name] = index
	}
	actualIndex := map[string]int{}
	for index, column := range actual.Columns {
		actualIndex[column.Name] = index
	}
	retentionExpected, ok := expectedIndex[invariant.Column]
	if !ok {
		return fail(ProblemInvariant, "expected table lacks retention column %q", invariant.Column)
	}
	retentionActual, ok := actualIndex[invariant.Column]
	if !ok {
		return fail(ProblemInvariant, "observed table lacks retention column %q", invariant.Column)
	}

	expectedGroups := map[string]*big.Rat{}
	for _, row := range expected.Rows {
		key, err := groupKey(row, invariant.GroupBy, expectedIndex)
		if err != nil {
			return fail(ProblemInvariant, "%s", err)
		}
		retention, ok := valueAt(row, retentionExpected)
		if !ok {
			return fail(ProblemRowShape, "expected table %q has a row narrower than its declared columns", invariant.Table)
		}
		value, ok := numericRat(retention)
		if !ok {
			return fail(ProblemInvariant, "expected retention value is not numeric")
		}
		if existing, exists := expectedGroups[key]; exists {
			if value.Cmp(existing) < 0 {
				expectedGroups[key] = value
			}
		} else {
			expectedGroups[key] = value
		}
	}

	actualGroups := map[string][][]apicontract.TypedValue{}
	for _, row := range actual.Rows {
		key, err := groupKey(row, invariant.GroupBy, actualIndex)
		if err != nil {
			return fail(ProblemInvariant, "%s", err)
		}
		actualGroups[key] = append(actualGroups[key], row)
	}

	for key, expectedMin := range expectedGroups {
		rows, ok := actualGroups[key]
		if !ok {
			return fail(ProblemInvariant, "expected surviving group is missing; the lowest %s must be preserved", invariant.Column)
		}
		if len(rows) != 1 {
			return fail(ProblemInvariant, "group has %d surviving rows; exactly one lowest-%s row is required", len(rows), invariant.Column)
		}
		retention, ok := valueAt(rows[0], retentionActual)
		if !ok {
			return fail(ProblemRowShape, "observed table %q has a row narrower than its declared columns", invariant.Table)
		}
		actualValue, ok := numericRat(retention)
		if !ok {
			return fail(ProblemInvariant, "observed retention value is not numeric")
		}
		if actualValue.Cmp(expectedMin) != 0 {
			return fail(ProblemInvariant, "group keeps %s %s but the lowest is %s; %s",
				invariant.Column, renderValue(retention), expectedMin.RatString(), invariant.Description)
		}
	}
	if len(actualGroups) != len(expectedGroups) {
		return fail(ProblemInvariant, "observed %d retention groups but expected %d; unrelated or extra rows must be preserved", len(actualGroups), len(expectedGroups))
	}
	return InvariantResult{ID: invariant.ID, Pass: true}
}
