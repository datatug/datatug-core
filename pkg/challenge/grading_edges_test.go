package challenge

import (
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/require"
)

func TestCompareTypedRowsRefusesInvalidOptionsAndShapes(t *testing.T) {
	columns := columnsOf("integer")
	rows := [][]apicontract.TypedValue{row(integer("1"))}

	for name, options := range map[string]RowOptions{
		"negative tolerance":  {Ordering: OrderUnordered, NumericTolerance: "-1"},
		"oversized tolerance": {Ordering: OrderUnordered, NumericTolerance: strings.Repeat("1", 33)},
	} {
		t.Run(name, func(t *testing.T) {
			diff := CompareTypedRows(columns, columns, rows, rows, options)
			require.False(t, diff.Match)
			require.Equal(t, ProblemInvalidOptions, diff.Problem)
		})
	}

	for name, test := range map[string]struct {
		compare     func() RowDiff
		wantProblem string
		wantMessage string
	}{
		"no expected columns": {
			compare: func() RowDiff {
				return CompareTypedRows(nil, columns, rows, rows, RowOptions{Ordering: OrderUnordered})
			},
		},
		"duplicate expected column names": {
			compare: func() RowDiff {
				// Both sides have two valid columns and two-cell rows, so only
				// the duplicated expected name can refuse the comparison.
				expectedColumns := []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "id", Type: "integer"}}
				observedColumns := []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "value", Type: "integer"}}
				validRows := [][]apicontract.TypedValue{row(integer("1"), integer("2"))}
				return CompareTypedRows(expectedColumns, observedColumns, validRows, validRows, RowOptions{Ordering: OrderUnordered})
			},
			wantProblem: ProblemInvalidOptions,
			wantMessage: `duplicate expected column name "id"`,
		},
		"no observed columns": {
			compare: func() RowDiff {
				return CompareTypedRows(columns, nil, rows, rows, RowOptions{Ordering: OrderUnordered})
			},
		},
		"too many expected rows": {
			compare: func() RowDiff {
				many := make([][]apicontract.TypedValue, MaxResultRows+1)
				for index := range many {
					many[index] = row(integer("1"))
				}
				return CompareTypedRows(columns, columns, many, nil, RowOptions{Ordering: OrderUnordered})
			},
		},
		"ordered row count differs": {
			compare: func() RowDiff {
				return CompareTypedRows(columns, columns, rows,
					[][]apicontract.TypedValue{{integer("1")}, {integer("2")}}, RowOptions{Ordering: OrderOrdered})
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			diff := test.compare()
			require.False(t, diff.Match)
			require.NotEmpty(t, diff.Problem)
			if test.wantProblem != "" {
				require.Equal(t, test.wantProblem, diff.Problem)
			}
			if test.wantMessage != "" {
				require.Contains(t, diff.Message, test.wantMessage)
			}
		})
	}
}

func TestCompareTypedRowsComparesNonNumericAndToleratedCells(t *testing.T) {
	booleanColumns := columnsOf("boolean")
	expected := [][]apicontract.TypedValue{{apicontract.NewBooleanValue(true)}}
	require.True(t, CompareTypedRows(booleanColumns, booleanColumns, expected, expected, RowOptions{Ordering: OrderUnordered}).Match)

	diff := CompareTypedRows(booleanColumns, booleanColumns, expected,
		[][]apicontract.TypedValue{{apicontract.NewBooleanValue(false)}}, RowOptions{Ordering: OrderUnordered})
	require.False(t, diff.Match)
	require.Equal(t, ProblemRowContent, diff.Problem)

	// A numeric tolerance on same-storage-class INTEGER cells compares values.
	integerColumns := columnsOf("integer")
	integerRows := [][]apicontract.TypedValue{{integer("2")}}
	require.True(t, CompareTypedRows(integerColumns, integerColumns, integerRows, integerRows,
		RowOptions{Ordering: OrderUnordered, NumericTolerance: "0.01"}).Match)

	tolerance, err := parseTolerance("0.01")
	require.NoError(t, err)
	require.False(t, typedValueEqual(integer("not-a-number"), number(1), tolerance),
		"an unparsable numeric cell must not be treated as within tolerance")
	require.False(t, typedRowsEqual(row(integer("1")), row(integer("1"), integer("2")), nil))
}

func TestFeedbackHelpersNeverReturnUnrenderableOrUnboundedExamples(t *testing.T) {
	require.Equal(t, "<unrenderable>", renderRow([]apicontract.TypedValue{{Type: "unsupported"}}))
	require.Equal(t, "<unrenderable>", renderValue(apicontract.TypedValue{Type: "unsupported"}))

	examples := boundedExamples([]string{"one", "two", "three", "four"})
	require.Equal(t, []string{"one", "two", "three"}, examples,
		"the helper must cap an overlong caller-supplied list deterministically")
	require.Nil(t, boundedExamples(nil))
}

func TestGradeSQLPartReportsUnavailableEvidenceInsteadOfGuessing(t *testing.T) {
	columns := columnsOf("integer")
	part := feedbackPart(columns, [][]apicontract.TypedValue{row(integer("1"))}, OrderUnordered)
	observations := map[string]Observation{"fixture": {Rows: checkRows(columns, 1)}}

	t.Run("empty answer", func(t *testing.T) {
		result := GradeSQLPart(part, "", observations)
		require.Equal(t, SQLPartResult{
			Outcome: OutcomeUnknown, ExecutionStatus: StatusError,
			Problem: ProblemSQLEmpty, Message: "SQL answer is empty",
		}, result)
	})

	t.Run("oversized answer", func(t *testing.T) {
		result := GradeSQLPart(part, strings.Repeat("x", MaxSQLBytes+1), observations)
		require.Equal(t, OutcomeUnknown, result.Outcome)
		require.Equal(t, StatusError, result.ExecutionStatus)
		require.Equal(t, ProblemSQLBytes, result.Problem)
	})

	t.Run("missing observation", func(t *testing.T) {
		result := GradeSQLPart(part, "SELECT 1", map[string]Observation{})
		require.Equal(t, OutcomeUnknown, result.Outcome)
		require.Equal(t, StatusError, result.ExecutionStatus)
		require.Equal(t, ProblemObservation, result.Problem)
		require.Contains(t, result.Message, `fixture "fixture"`)
		require.Len(t, result.Cases, 1)
		require.Equal(t, OutcomeUnknown, result.Cases[0].Outcome)
		require.Equal(t, StatusError, result.Cases[0].ExecutionStatus)
	})

	t.Run("execution error", func(t *testing.T) {
		result := GradeSQLPart(part, "SELECT 1", map[string]Observation{
			"fixture": {Rows: checkRows(columns, 1), ErrorCode: "SQLITE_BUSY"},
		})
		require.Equal(t, OutcomeUnknown, result.Outcome)
		require.Equal(t, StatusError, result.ExecutionStatus)
		require.Equal(t, ProblemExecutionError, result.Problem)
		require.Contains(t, result.Message, "SQLITE_BUSY")
	})

	t.Run("result oracle without a recordset", func(t *testing.T) {
		result := GradeSQLPart(part, "SELECT 1", map[string]Observation{"fixture": {}})
		require.Equal(t, OutcomeUnknown, result.Outcome)
		require.Equal(t, ProblemObservation, result.Problem)
		require.Contains(t, result.Message, "returned no recordset")
	})

	t.Run("invalid oracle", func(t *testing.T) {
		invalid := part
		invalid.Cases = append([]SQLCase(nil), part.Cases...)
		invalid.Cases[0].Oracle = Oracle{Kind: "bogus"}
		result := GradeSQLPart(invalid, "SELECT 1", observations)
		require.Equal(t, OutcomeUnknown, result.Outcome)
		require.Equal(t, StatusError, result.ExecutionStatus)
		require.Equal(t, ProblemOracleInvalid, result.Problem)
		require.Contains(t, result.Message, "unknown oracle kind")
	})

	t.Run("final-state oracle without tables", func(t *testing.T) {
		finalState := reviewFinalStateOracle()
		finalPart := SQLPart{Cases: []SQLCase{{
			Fixture: "customers",
			Oracle:  Oracle{Kind: OracleFinalState, FinalState: &finalState},
		}}}
		result := GradeSQLPart(finalPart, "DELETE FROM customers;", map[string]Observation{"customers": {}})
		require.Equal(t, OutcomeUnknown, result.Outcome)
		require.Equal(t, StatusError, result.ExecutionStatus)
		require.Equal(t, ProblemObservation, result.Problem)
		require.Contains(t, result.Message, "returned no final-state tables")
	})

	t.Run("a later missing observation does not erase an earlier pass", func(t *testing.T) {
		twoCases := feedbackPart(columns, [][]apicontract.TypedValue{row(integer("1"))}, OrderUnordered)
		missing := twoCases.Cases[0]
		missing.Fixture = "missing"
		twoCases.Cases = append(twoCases.Cases, missing)

		result := GradeSQLPart(twoCases, "SELECT 1", map[string]Observation{
			"fixture": {Rows: &apicontract.Recordset{Columns: columns, Rows: [][]apicontract.TypedValue{row(integer("1"))}}},
		})
		require.Equal(t, OutcomeUnknown, result.Outcome)
		require.Equal(t, StatusError, result.ExecutionStatus)
		require.Equal(t, ProblemObservation, result.Problem)
		require.Contains(t, result.Message, `fixture "missing"`)
		require.Len(t, result.Cases, 2)
		require.Equal(t, OutcomePass, result.Cases[0].Outcome)
		require.Equal(t, OutcomeUnknown, result.Cases[1].Outcome)
	})
}

func TestEvaluateFinalStateReportsEveryStructuralProblem(t *testing.T) {
	columns := reviewColumns()
	expected := reviewFinalStateOracle()
	passing := []TableState{{
		Name: "customers", Columns: columns,
		Rows: [][]apicontract.TypedValue{{text("a@x.com"), integer("1")}},
	}}

	t.Run("pass", func(t *testing.T) {
		result := EvaluateFinalState(expected, passing)
		require.True(t, result.Pass)
		require.Empty(t, result.ViolatedInvariant())
		require.Len(t, result.Tables, 1)
		require.True(t, result.Tables[0].Pass)
		require.Len(t, result.Invariants, 1)
		require.True(t, result.Invariants[0].Pass)
	})

	t.Run("duplicate observed table", func(t *testing.T) {
		result := EvaluateFinalState(expected, append(append([]TableState(nil), passing...), passing[0]))
		require.False(t, result.Pass)
		require.Contains(t, result.Tables, TableResult{
			Table: "customers", Problem: ProblemRowContent, Message: "duplicate observed table",
		})
	})

	t.Run("expected table is unavailable", func(t *testing.T) {
		result := EvaluateFinalState(expected, nil)
		require.False(t, result.Pass)
		require.Contains(t, result.Tables, TableResult{
			Table: "customers", Problem: ProblemRowContent, Message: "expected final-state table is unavailable",
		})
		require.False(t, result.Invariants[0].Pass)
		require.Contains(t, result.Invariants[0].Message, "is unavailable")
	})

	t.Run("unexpected observed table", func(t *testing.T) {
		extra := TableState{Name: "extra", Columns: columns, Rows: [][]apicontract.TypedValue{{text("b@x.com"), integer("2")}}}
		result := EvaluateFinalState(expected, append(append([]TableState(nil), passing...), extra))
		require.False(t, result.Pass)
		require.Equal(t, ProblemRowContent, result.Problem)
		require.Equal(t, `unexpected observed table "extra"`, result.Message)
		require.Contains(t, result.Tables, TableResult{
			Table: "extra", Problem: ProblemRowContent, Message: "observed table is not declared by the final-state oracle",
		})
	})

	t.Run("unexpected retention group", func(t *testing.T) {
		actual := []TableState{{
			Name: "customers", Columns: columns,
			Rows: [][]apicontract.TypedValue{
				{text("a@x.com"), integer("1")},
				{text("b@x.com"), integer("2")},
			},
		}}
		result := EvaluateFinalState(expected, actual)
		require.False(t, result.Pass)
		require.Equal(t, ProblemInvariant, result.Problem)
		require.Contains(t, result.Invariants[0].Message, "observed 2 retention groups but expected 1")
	})

	t.Run("expected table lacks retention column", func(t *testing.T) {
		oracle := FinalStateOracle{
			Tables: []TableExpectation{{
				Name:    "customers",
				Columns: []apicontract.Column{{Name: "email", Type: "string"}},
				Rows:    [][]apicontract.TypedValue{{text("a@x.com")}},
			}},
			Invariants: []Invariant{reviewInvariant()},
		}
		result := EvaluateFinalState(oracle, []TableState{{
			Name: "customers", Columns: oracle.Tables[0].Columns, Rows: oracle.Tables[0].Rows,
		}})
		require.False(t, result.Pass)
		require.Contains(t, result.Invariants[0].Message, `expected table lacks retention column "id"`)
	})

	t.Run("observed table lacks retention column", func(t *testing.T) {
		actual := []TableState{{
			Name:    "customers",
			Columns: []apicontract.Column{{Name: "email", Type: "string"}, {Name: "other", Type: "integer"}},
			Rows:    [][]apicontract.TypedValue{{text("a@x.com"), integer("1")}},
		}}
		result := EvaluateFinalState(expected, actual)
		require.False(t, result.Pass)
		require.Contains(t, result.Invariants[0].Message, `observed table lacks retention column "id"`)
	})

	t.Run("expected table lacks the group column", func(t *testing.T) {
		invariant := reviewInvariant()
		invariant.GroupBy = []string{"missing"}
		oracle := expected
		oracle.Invariants = []Invariant{invariant}
		result := EvaluateFinalState(oracle, passing)
		require.False(t, result.Pass)
		require.Contains(t, result.Invariants[0].Message, `unknown group column "missing"`)
	})

	t.Run("observed table lacks the group column", func(t *testing.T) {
		actual := []TableState{{
			Name:    "customers",
			Columns: []apicontract.Column{{Name: "other", Type: "string"}, {Name: "id", Type: "integer"}},
			Rows:    [][]apicontract.TypedValue{{text("a@x.com"), integer("1")}},
		}}
		result := EvaluateFinalState(expected, actual)
		require.False(t, result.Pass)
		require.Contains(t, result.Invariants[0].Message, `unknown group column "email"`)
	})

	t.Run("expected retention value is not numeric", func(t *testing.T) {
		oracle := FinalStateOracle{
			Tables: []TableExpectation{{
				Name:    "customers",
				Columns: reviewColumns(),
				Rows:    [][]apicontract.TypedValue{{text("a@x.com"), text("not-a-number")}},
			}},
			Invariants: []Invariant{reviewInvariant()},
		}
		result := EvaluateFinalState(oracle, []TableState{{
			Name: "customers", Columns: oracle.Tables[0].Columns, Rows: oracle.Tables[0].Rows,
		}})
		require.False(t, result.Pass)
		require.Contains(t, result.Invariants[0].Message, "expected retention value is not numeric")
	})

	t.Run("observed retention value is not numeric", func(t *testing.T) {
		actual := []TableState{{
			Name: "customers", Columns: columns,
			Rows: [][]apicontract.TypedValue{{text("a@x.com"), text("not-a-number")}},
		}}
		result := EvaluateFinalState(expected, actual)
		require.False(t, result.Pass)
		require.Contains(t, result.Invariants[0].Message, "observed retention value is not numeric")
	})

	t.Run("expected groups reduce to their lowest retention value", func(t *testing.T) {
		oracle := FinalStateOracle{
			Tables: []TableExpectation{{
				Name:    "customers",
				Columns: reviewColumns(),
				Rows: [][]apicontract.TypedValue{
					{text("a@x.com"), integer("2")},
					{text("a@x.com"), integer("1")},
				},
			}},
			Invariants: []Invariant{reviewInvariant()},
		}
		result := EvaluateFinalState(oracle, passing)
		require.False(t, result.Pass, "the table itself has the wrong row count")
		require.True(t, result.Invariants[0].Pass, "the surviving row is the lowest expected retention value")
	})
}

func TestGradeAttemptRefusesUnsupportedOutcomeVocabulary(t *testing.T) {
	opened := openGolden(t, "retry-evidence")
	manifest := opened.Manifest
	sqlPart := findPart(t, manifest, "requests")
	structuredPart := findPart(t, manifest, "evidence-pairs")
	sqlOutcome := SQLPartResult{Outcome: OutcomePass, ExecutionStatus: StatusCompleted}.AsOutcome(sqlPart)
	structuredOutcome := StructuredResult{Outcome: OutcomePass, ExecutionStatus: StatusCompleted}.AsOutcome(structuredPart)

	unknownOutcome := sqlOutcome
	unknownOutcome.Outcome = "maybe"
	_, err := GradeAttempt(manifest, []PartOutcome{unknownOutcome, structuredOutcome})
	_ = requireRefused(t, err, CodeInvalidAnswer)

	unknownStatus := sqlOutcome
	unknownStatus.ExecutionStatus = "partial"
	_, err = GradeAttempt(manifest, []PartOutcome{unknownStatus, structuredOutcome})
	_ = requireRefused(t, err, CodeInvalidAnswer)
}
