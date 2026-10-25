package challenge

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/require"
)

func reviewColumns() []apicontract.Column {
	return []apicontract.Column{{Name: "email", Type: "string"}, {Name: "id", Type: "integer"}}
}

func reviewInvariant() Invariant {
	return Invariant{
		ID: "keep-lowest-id", Kind: InvariantGroupMin, Table: "customers",
		GroupBy: []string{"email"}, Column: "id",
		Description: "Keep the lowest id per exact email value.",
	}
}

func reviewInvariants(count int) []Invariant {
	invariants := make([]Invariant, count)
	for index := range invariants {
		invariants[index] = reviewInvariant()
		invariants[index].ID = fmt.Sprintf("keep-lowest-id-%02d", index)
	}
	return invariants
}

func reviewFinalStateOracle() FinalStateOracle {
	return FinalStateOracle{
		Tables: []TableExpectation{{
			Name:    "customers",
			Columns: reviewColumns(),
			Rows:    [][]apicontract.TypedValue{{text("a@x.com"), integer("1")}},
		}},
		Invariants: []Invariant{reviewInvariant()},
	}
}

func TestOracleWireFormRefusesUnknownAndAmbiguousShapes(t *testing.T) {
	t.Run("probe is not an object", func(t *testing.T) {
		var oracle Oracle
		require.Error(t, json.Unmarshal([]byte(`[]`), &oracle))
		var malformed Oracle
		require.Error(t, json.Unmarshal([]byte(`{"kind":`), &malformed))
	})

	t.Run("unknown kind", func(t *testing.T) {
		var oracle Oracle
		err := json.Unmarshal([]byte(`{"kind":"magic"}`), &oracle)
		require.ErrorContains(t, err, OracleResult)
		require.ErrorContains(t, err, OracleFinalState)
	})

	t.Run("final-state unknown field", func(t *testing.T) {
		var oracle Oracle
		err := json.Unmarshal([]byte(`{"kind":"final-state","tables":[],"unexpected":true}`), &oracle)
		require.Error(t, err)
	})

	for name, oracle := range map[string]Oracle{
		"result without a body": {
			Kind: OracleResult,
		},
		"result with two bodies": {
			Kind: OracleResult, Result: &ResultOracle{}, FinalState: &FinalStateOracle{},
		},
		"final-state without a body": {
			Kind: OracleFinalState,
		},
		"final-state with two bodies": {
			Kind: OracleFinalState, Result: &ResultOracle{}, FinalState: &FinalStateOracle{},
		},
	} {
		t.Run("marshal "+name, func(t *testing.T) {
			_, err := oracle.MarshalJSON()
			require.ErrorContains(t, err, "missing or ambiguous")
		})
	}

	_, err := Oracle{Kind: "magic"}.MarshalJSON()
	require.ErrorContains(t, err, "unknown oracle kind")

	require.Error(t, Oracle{Kind: OracleResult}.Validate())
	require.Error(t, Oracle{Kind: OracleFinalState}.Validate())
	require.Error(t, Oracle{Kind: "magic"}.Validate())
}

func TestResultOracleValidationRejectsBadOptionsAndCells(t *testing.T) {
	valid := func() ResultOracle {
		return ResultOracle{
			Columns:  []apicontract.Column{{Name: "id", Type: "integer"}},
			Rows:     [][]apicontract.TypedValue{{integer("1")}},
			Ordering: OrderUnordered,
		}
	}
	require.NoError(t, valid().Validate())

	for name, test := range map[string]struct {
		mutate func(*ResultOracle)
		want   string
	}{
		"unknown ordering":   {mutate: func(oracle *ResultOracle) { oracle.Ordering = "sideways" }},
		"negative tolerance": {mutate: func(oracle *ResultOracle) { oracle.NumericTolerance = "-1" }},
		"no columns":         {mutate: func(oracle *ResultOracle) { oracle.Columns = nil }},
		"duplicate column names": {
			mutate: func(oracle *ResultOracle) {
				oracle.Columns = []apicontract.Column{
					{Name: "id", Type: "integer"},
					{Name: "id", Type: "integer"},
				}
				// The rows stay valid two-cell rows, so only the duplicate
				// column name can refuse this oracle.
				oracle.Rows = [][]apicontract.TypedValue{{integer("1"), integer("2")}}
			},
			want: `duplicate expected column name "id"`,
		},
		"too many rows": {
			mutate: func(oracle *ResultOracle) {
				oracle.Rows = make([][]apicontract.TypedValue, MaxResultRows+1)
				for index := range oracle.Rows {
					oracle.Rows[index] = row(integer("1"))
				}
			},
		},
		"malformed cell": {
			mutate: func(oracle *ResultOracle) {
				oracle.Rows = [][]apicontract.TypedValue{{integer("+1")}}
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			oracle := valid()
			test.mutate(&oracle)
			err := oracle.Validate()
			require.Error(t, err)
			if test.want != "" {
				require.ErrorContains(t, err, test.want)
			}
		})
	}
}

func TestTableExpectationValidationRejectsBadNamesAndShapes(t *testing.T) {
	require.NoError(t, TableExpectation{
		Name:    "customers",
		Columns: []apicontract.Column{{Name: "id", Type: "integer"}},
		Rows:    [][]apicontract.TypedValue{{integer("1")}},
	}.Validate())

	require.ErrorContains(t, TableExpectation{Name: ""}.Validate(), "invalid table name")
	require.ErrorContains(t, TableExpectation{
		Name: "customers",
		Rows: [][]apicontract.TypedValue{{integer("1")}},
	}.Validate(), "column count")
}

func TestFinalStateOracleValidationRejectsUnusableExpectations(t *testing.T) {
	require.NoError(t, reviewFinalStateOracle().Validate())

	validTable := reviewFinalStateOracle().Tables[0]

	// The overflow case must be refused only by the count cap: every invariant
	// is otherwise valid and carries a distinct id.
	overflow := reviewInvariants(maxManifestParts + 1)
	overflowTables := map[string]TableExpectation{"customers": validTable}
	overflowIDs := map[string]bool{}
	for _, invariant := range overflow {
		require.False(t, overflowIDs[invariant.ID], "overflow invariant ids must be distinct")
		overflowIDs[invariant.ID] = true
		require.NoError(t, invariant.Validate(overflowTables), "overflow invariants must be individually valid")
	}

	for name, test := range map[string]struct {
		oracle FinalStateOracle
		want   string
	}{
		"no tables": {oracle: FinalStateOracle{}},
		"duplicate table names": {
			oracle: FinalStateOracle{Tables: []TableExpectation{validTable, validTable}},
		},
		"table shape is invalid": {
			oracle: FinalStateOracle{Tables: []TableExpectation{{Name: "customers"}}},
		},
		"too many invariants": {
			oracle: FinalStateOracle{
				Tables:     []TableExpectation{validTable},
				Invariants: overflow,
			},
			want: "too many final-state invariants",
		},
		"duplicate invariant ids": {
			oracle: FinalStateOracle{
				Tables:     []TableExpectation{validTable},
				Invariants: []Invariant{reviewInvariant(), reviewInvariant()},
			},
		},
		"retention groups have several rows": {
			oracle: FinalStateOracle{
				Tables: []TableExpectation{{
					Name:    "customers",
					Columns: reviewColumns(),
					Rows: [][]apicontract.TypedValue{
						{text("a@x.com"), integer("1")},
						{text("a@x.com"), integer("2")},
					},
				}},
				Invariants: []Invariant{reviewInvariant()},
			},
		},
		"retention value is null": {
			oracle: FinalStateOracle{
				Tables: []TableExpectation{{
					Name:    "customers",
					Columns: reviewColumns(),
					Rows:    [][]apicontract.TypedValue{{text("a@x.com"), null()}},
				}},
				Invariants: []Invariant{reviewInvariant()},
			},
		},
		"retention value is text": {
			oracle: FinalStateOracle{
				Tables: []TableExpectation{{
					Name:    "customers",
					Columns: reviewColumns(),
					Rows:    [][]apicontract.TypedValue{{text("a@x.com"), text("1")}},
				}},
				Invariants: []Invariant{reviewInvariant()},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := test.oracle.Validate()
			require.Error(t, err)
			if test.want != "" {
				require.ErrorContains(t, err, test.want)
			}
		})
	}

	err := FinalStateOracle{Tables: []TableExpectation{{Name: "customers"}}}.Validate()
	require.ErrorContains(t, err, `table "customers"`)
}

func TestInvariantValidationRejectsUnsupportedRetentionRules(t *testing.T) {
	columns := reviewColumns()
	validTable := TableExpectation{
		Name:    "customers",
		Columns: columns,
		Rows:    [][]apicontract.TypedValue{{text("a@x.com"), integer("1")}},
	}
	tables := map[string]TableExpectation{"customers": validTable}
	invariant := reviewInvariant()
	require.NoError(t, invariant.Validate(tables))

	for name, mutate := range map[string]func(*Invariant){
		"invalid id":               func(value *Invariant) { value.ID = "Bad_ID" },
		"unsupported kind":         func(value *Invariant) { value.Kind = "group-max" },
		"empty description":        func(value *Invariant) { value.Description = "" },
		"missing table":            func(value *Invariant) { value.Table = "missing" },
		"no group columns":         func(value *Invariant) { value.GroupBy = nil },
		"unknown group column":     func(value *Invariant) { value.GroupBy = []string{"missing"} },
		"duplicate group column":   func(value *Invariant) { value.GroupBy = []string{"email", "email"} },
		"group by retention":       func(value *Invariant) { value.GroupBy = []string{"id"} },
		"unknown retention column": func(value *Invariant) { value.Column = "missing" },
	} {
		t.Run(name, func(t *testing.T) {
			mutated := invariant
			mutated.GroupBy = append([]string(nil), invariant.GroupBy...)
			mutate(&mutated)
			require.Error(t, mutated.Validate(tables))
		})
	}

	// A direct table map can be narrower than its declared columns. Retention
	// evaluation must report a rule failure instead of indexing past the row.
	for name, malformed := range map[string]TableExpectation{
		"row missing the group column": {
			Name:    "customers",
			Columns: []apicontract.Column{{Name: "email", Type: "string"}, {Name: "note", Type: "string"}, {Name: "id", Type: "integer"}},
			Rows:    [][]apicontract.TypedValue{{}},
		},
		"row missing the retention column": {
			Name:    "customers",
			Columns: []apicontract.Column{{Name: "email", Type: "string"}, {Name: "note", Type: "string"}, {Name: "id", Type: "integer"}},
			Rows:    [][]apicontract.TypedValue{{text("a@x.com")}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, reviewInvariant().Validate(map[string]TableExpectation{"customers": malformed}))
		})
	}
}

func TestTypedRowHelpersRefuseUnindexableAndUnrenderableCells(t *testing.T) {
	cells := row(integer("1"), text("a@x.com"))
	value, ok := valueAt(cells, -1)
	require.False(t, ok)
	require.Equal(t, apicontract.TypedValue{}, value)
	value, ok = valueAt(cells, len(cells))
	require.False(t, ok)
	require.Equal(t, apicontract.TypedValue{}, value)
	value, ok = valueAt(cells, 1)
	require.True(t, ok)
	require.Equal(t, text("a@x.com"), value)

	index := map[string]int{"id": 0, "email": 1}
	key, err := groupKey(row(integer("1"), text("a@x.com")), []string{"id", "email"}, index)
	require.NoError(t, err)
	sameKey, err := groupKey(row(integer("1"), text("a@x.com")), []string{"id", "email"}, map[string]int{"id": 0, "email": 1})
	require.NoError(t, err)
	require.Equal(t, key, sameKey)
	differentKey, err := groupKey(row(integer("1"), text("b@x.com")), []string{"id", "email"}, index)
	require.NoError(t, err)
	require.NotEqual(t, key, differentKey)

	_, err = groupKey(cells, []string{"missing"}, index)
	require.ErrorContains(t, err, `unknown group column "missing"`)
	_, err = groupKey(row(integer("1")), []string{"email"}, index)
	require.ErrorContains(t, err, "is unavailable")
	_, err = groupKey(row(apicontract.TypedValue{Type: "unsupported"}), []string{"id"}, index)
	require.ErrorContains(t, err, "TypedValue")
}

func TestStructuredValidationCoversEveryClosedFieldRule(t *testing.T) {
	validField := StructuredField{
		ID:            "evidence",
		Label:         "Evidence",
		Cardinality:   CardinalityMulti,
		Required:      true,
		MinSelections: 1,
		MaxSelections: 2,
		Options: []StructuredOption{
			{ID: "a", Label: "A"},
			{ID: "b", Label: "B"},
			{ID: "c", Label: "C"},
		},
	}
	valid := StructuredPart{
		Fields:   []StructuredField{validField},
		Expected: StructuredAnswer{Selections: map[string][]string{"evidence": {"a"}}},
	}
	require.NoError(t, valid.Validate())

	for name, part := range map[string]StructuredPart{
		"no fields": {},
		"unknown cardinality": {
			Fields: []StructuredField{{
				ID: "f", Label: "F", Cardinality: "many",
				Options: []StructuredOption{{ID: "a", Label: "A"}},
			}},
		},
		"no options": {
			Fields: []StructuredField{{ID: "f", Label: "F", Cardinality: CardinalitySingle}},
		},
		"duplicate option": {
			Fields: []StructuredField{{
				ID: "f", Label: "F", Cardinality: CardinalitySingle,
				Options: []StructuredOption{{ID: "a", Label: "A"}, {ID: "a", Label: "B"}},
			}},
		},
		"single field declares a range": {
			Fields: []StructuredField{{
				ID: "f", Label: "F", Cardinality: CardinalitySingle, MinSelections: 1,
				Options: []StructuredOption{{ID: "a", Label: "A"}},
			}},
		},
		"required field has no expected selection": {
			Fields: []StructuredField{{
				ID: "f", Label: "F", Cardinality: CardinalityMulti, Required: true,
				MinSelections: 1, MaxSelections: 1,
				Options: []StructuredOption{{ID: "a", Label: "A"}},
			}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, part.Validate())
		})
	}
}

func TestEvaluateStructuredTreatsOptionalFieldsAsTrulyOptional(t *testing.T) {
	part := StructuredPart{
		Fields: []StructuredField{{
			ID: "note", Label: "Note", Cardinality: CardinalityMulti,
			Required: false, MinSelections: 0, MaxSelections: 1,
			Options: []StructuredOption{{ID: "a", Label: "A"}},
		}},
		Expected: StructuredAnswer{Selections: map[string][]string{}},
	}
	require.NoError(t, part.Validate())

	result := EvaluateStructured(part, StructuredAnswer{Selections: map[string][]string{}})
	require.Equal(t, OutcomePass, result.Outcome)
	require.Equal(t, StatusCompleted, result.ExecutionStatus)
	require.Equal(t, []FieldOutcome{{FieldID: "note", Outcome: OutcomePass}}, result.Fields)

	// An optional field with a declared expected answer still requires it.
	withExpected := part
	withExpected.Expected = StructuredAnswer{Selections: map[string][]string{"note": {"a"}}}
	result = EvaluateStructured(withExpected, StructuredAnswer{Selections: map[string][]string{}})
	require.Equal(t, OutcomeFail, result.Outcome)
	require.Equal(t, ProblemMissingSelection, result.Problem)
}

func TestEvaluateStructuredComparesSelectionSetsByLengthAndMembers(t *testing.T) {
	part := StructuredPart{
		Fields: []StructuredField{{
			ID: "evidence", Label: "Evidence", Cardinality: CardinalityMulti,
			Required: true, MinSelections: 1, MaxSelections: 2,
			Options: []StructuredOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		}},
		Expected: StructuredAnswer{Selections: map[string][]string{"evidence": {"a", "b"}}},
	}
	result := EvaluateStructured(part, StructuredAnswer{Selections: map[string][]string{"evidence": {"a"}}})
	require.Equal(t, OutcomeFail, result.Outcome)
	require.Equal(t, ProblemSelectionMismatch, result.Fields[0].Problem)
	require.Contains(t, result.Message, "expected [a b], selected [a]")
}
