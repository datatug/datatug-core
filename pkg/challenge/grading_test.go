package challenge

import (
	"encoding/json"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/datatug/datatug-core/pkg/investigation"
	"github.com/stretchr/testify/require"
)

func integerExpectation(value string) detective.Validator {
	expected := investigation.NewIntegerValue(value)
	return detective.Validator{ID: "no-duplicates", Scope: "set", Kind: "row-count", Severity: "critical", Expected: &expected}
}

func retryStructuredPart() Part {
	return Part{
		ID:         "evidence-pairs",
		Title:      "Select the duplicate request evidence",
		Required:   true,
		AnswerKind: AnswerStructured,
		Structured: &StructuredPart{
			Fields: []StructuredField{{
				ID:            "evidence-pairs",
				Label:         "Evidence pairs",
				Cardinality:   CardinalityMulti,
				Required:      true,
				MinSelections: 2,
				MaxSelections: 2,
				Options: []StructuredOption{
					{ID: "r1-a1-trace", Label: "r1"},
					{ID: "r2-a2-a3-trace", Label: "r2"},
					{ID: "r3-a4-a5-trace", Label: "r3"},
					{ID: "r4-a6-a7-trace", Label: "r4"},
				},
			}},
			Expected: StructuredAnswer{Selections: map[string][]string{
				"evidence-pairs": {"r4-a6-a7-trace", "r2-a2-a3-trace"},
			}},
		},
	}
}

func TestStructuredAnswerUsesExactSetEquality(t *testing.T) {
	part := retryStructuredPart()
	require.NoError(t, part.Validate())

	correct := StructuredAnswer{Selections: map[string][]string{
		"evidence-pairs": {"r2-a2-a3-trace", "r4-a6-a7-trace"},
	}}
	result := EvaluateStructured(*part.Structured, correct)
	require.Equal(t, OutcomePass, result.Outcome)
	require.Equal(t, StatusCompleted, result.ExecutionStatus)
	require.Len(t, result.Fields, 1)

	wrong := StructuredAnswer{Selections: map[string][]string{
		"evidence-pairs": {"r2-a2-a3-trace", "r3-a4-a5-trace"},
	}}
	result = EvaluateStructured(*part.Structured, wrong)
	require.Equal(t, OutcomeFail, result.Outcome)
	require.Equal(t, ProblemSelectionMismatch, result.Fields[0].Problem)
	require.Equal(t, "evidence-pairs", result.Fields[0].FieldID)

	missing := StructuredAnswer{Selections: map[string][]string{}}
	result = EvaluateStructured(*part.Structured, missing)
	require.Equal(t, OutcomeFail, result.Outcome)
	require.Equal(t, ProblemMissingSelection, result.Fields[0].Problem)

	duplicate := StructuredAnswer{Selections: map[string][]string{
		"evidence-pairs": {"r2-a2-a3-trace", "r2-a2-a3-trace"},
	}}
	result = EvaluateStructured(*part.Structured, duplicate)
	require.Equal(t, StatusError, result.ExecutionStatus)
	require.Equal(t, OutcomeUnknown, result.Outcome)
	require.Equal(t, ProblemDuplicateOption, result.Problem)

	unknownOption := StructuredAnswer{Selections: map[string][]string{
		"evidence-pairs": {"r2-a2-a3-trace", "free-text"},
	}}
	result = EvaluateStructured(*part.Structured, unknownOption)
	require.Equal(t, StatusError, result.ExecutionStatus)
	require.Equal(t, ProblemUnknownOption, result.Problem)

	unknownField := StructuredAnswer{Selections: map[string][]string{
		"evidence-pairs": {"r2-a2-a3-trace", "r4-a6-a7-trace"},
		"free-text":      {"anything"},
	}}
	result = EvaluateStructured(*part.Structured, unknownField)
	require.Equal(t, StatusError, result.ExecutionStatus)
	require.Equal(t, OutcomeUnknown, result.Outcome)
	require.Equal(t, ProblemUnknownField, result.Problem)
}

func TestStructuredAnswerSingleSelectCardinality(t *testing.T) {
	part := StructuredPart{
		Fields: []StructuredField{{
			ID:          "cause",
			Label:       "Cause",
			Cardinality: CardinalitySingle,
			Required:    true,
			Options: []StructuredOption{
				{ID: "missing-idempotency", Label: "Missing idempotency"},
				{ID: "provider-double-send", Label: "Provider double send"},
			},
		}},
		Expected: StructuredAnswer{Selections: map[string][]string{"cause": {"missing-idempotency"}}},
	}
	require.NoError(t, part.Validate())
	require.Equal(t, OutcomePass, EvaluateStructured(part, StructuredAnswer{Selections: map[string][]string{"cause": {"missing-idempotency"}}}).Outcome)

	result := EvaluateStructured(part, StructuredAnswer{Selections: map[string][]string{"cause": {"missing-idempotency", "provider-double-send"}}})
	require.Equal(t, StatusError, result.ExecutionStatus)
	require.Equal(t, ProblemSingleCardinality, result.Problem)

	result = EvaluateStructured(part, StructuredAnswer{Selections: map[string][]string{"cause": {"provider-double-send"}}})
	require.Equal(t, OutcomeFail, result.Outcome)
}

func TestStructuredManifestValidationRejectsOpenWorldAnswers(t *testing.T) {
	part := retryStructuredPart()
	part.ID = "structured"
	require.NoError(t, part.Validate())

	bad := part
	bad.Structured = &StructuredPart{
		Fields: []StructuredField{{
			ID: "f", Label: "f", Cardinality: CardinalityMulti, Required: true, MinSelections: 1, MaxSelections: 1,
			Options: []StructuredOption{{ID: "a", Label: "a"}},
		}},
		Expected: StructuredAnswer{Selections: map[string][]string{"f": {"b"}}},
	}
	require.Error(t, bad.Validate())

	bad = part
	bad.Structured = &StructuredPart{
		Fields: []StructuredField{{
			ID: "f", Label: "f", Cardinality: CardinalityMulti, Required: true, MinSelections: 2, MaxSelections: 1,
			Options: []StructuredOption{{ID: "a", Label: "a"}},
		}},
		Expected: StructuredAnswer{Selections: map[string][]string{"f": {"a"}}},
	}
	require.Error(t, bad.Validate())
}

func checkPart() Part {
	return Part{
		ID:         "prevention-check",
		Title:      "Detect recurring duplicates",
		Required:   true,
		AnswerKind: AnswerCheckDefinition,
		CheckDefinition: &CheckDefinitionPart{
			QueryIDs:    []string{"queries/duplicate-emails"},
			Expectation: integerExpectation("0"),
			Cases: []CheckCase{
				{Fixture: "broken", ExpectedOutcome: OutcomeFail},
				{Fixture: "repaired", ExpectedOutcome: OutcomePass},
			},
		},
	}
}

func checkRows(columns []apicontract.Column, count int) *apicontract.Recordset {
	result := &apicontract.Recordset{Columns: columns}
	for index := 0; index < count; index++ {
		result.Rows = append(result.Rows, []apicontract.TypedValue{apicontract.NewStringValue("x")})
	}
	return result
}

func checkObservations(columns []apicontract.Column, brokenRows, repairedRows int) map[string]Observation {
	return map[string]Observation{
		"broken":   {Rows: checkRows(columns, brokenRows)},
		"repaired": {Rows: checkRows(columns, repairedRows)},
	}
}

func TestCheckDefinitionValidatesOutcomesOnBrokenAndRepairedFixtures(t *testing.T) {
	part := checkPart()
	require.NoError(t, part.Validate())
	answer := CheckDefinitionAnswer{QueryID: "queries/duplicate-emails", Expectation: part.CheckDefinition.Expectation}
	columns := []apicontract.Column{{Name: "email", Type: "string"}}

	result := EvaluateCheckDefinition(*part.CheckDefinition, answer, checkObservations(columns, 2, 0))
	require.Equal(t, OutcomePass, result.Outcome, result.Message)
	require.Len(t, result.Cases, 2)

	// An expectation that always passes accepts broken data and must not pass.
	wrongExpectation := answer
	wrongExpectation.Expectation = integerExpectation("100")
	result = EvaluateCheckDefinition(*part.CheckDefinition, wrongExpectation, checkObservations(columns, 2, 0))
	require.Equal(t, OutcomeFail, result.Outcome)
	require.Equal(t, ProblemExpectationMismatch, result.Problem)

	// A check that reports the same outcome for every fixture fails the case.
	result = EvaluateCheckDefinition(*part.CheckDefinition, answer, checkObservations(columns, 0, 0))
	require.Equal(t, OutcomeFail, result.Outcome)
	require.Equal(t, ProblemCaseOutcome, result.Problem)
	require.Equal(t, "broken", result.Cases[0].Fixture)

	// Missing observations are an execution error, never a silent pass.
	result = EvaluateCheckDefinition(*part.CheckDefinition, answer, map[string]Observation{"broken": {Rows: checkRows(columns, 2)}})
	require.Equal(t, StatusError, result.ExecutionStatus)
	require.Equal(t, OutcomeUnknown, result.Outcome)
	require.Equal(t, ProblemMissingObservation, result.Problem)

	// Closed query selection and supported expectation kinds.
	unknownQuery := answer
	unknownQuery.QueryID = "queries/elsewhere"
	result = EvaluateCheckDefinition(*part.CheckDefinition, unknownQuery, checkObservations(columns, 2, 0))
	require.Equal(t, StatusError, result.ExecutionStatus)
	require.Equal(t, ProblemUnknownQuery, result.Problem)

	unsupported := answer
	unsupported.Expectation = detective.Validator{ID: "unique-email", Scope: "set", Kind: "unique", Severity: "critical", Field: "email"}
	result = EvaluateCheckDefinition(*part.CheckDefinition, unsupported, checkObservations(columns, 2, 0))
	require.Equal(t, StatusError, result.ExecutionStatus)
	require.Equal(t, ProblemUnsupportedExpectation, result.Problem)

	// A Check definition that names a different saved query is invalid.
	badPart := part
	badPart.CheckDefinition = &CheckDefinitionPart{
		QueryIDs:    []string{"queries/duplicate-emails"},
		Expectation: integerExpectation("0"),
		Cases:       []CheckCase{{Fixture: "broken", ExpectedOutcome: OutcomeFail}},
	}
	require.NoError(t, badPart.Validate())
}

func TestCheckDefinitionTreatsFixtureExecutionErrorAsUnknown(t *testing.T) {
	part := checkPart()
	answer := CheckDefinitionAnswer{QueryID: "queries/duplicate-emails", Expectation: part.CheckDefinition.Expectation}
	columns := []apicontract.Column{{Name: "email", Type: "string"}}

	require.Equal(t, OutcomePass, EvaluateCheckDefinition(*part.CheckDefinition, answer, checkObservations(columns, 2, 0)).Outcome)

	// A failed execution whose partial rows happen to match the expectation
	// must not be graded as a pass: the error is checked before the rows.
	failed := checkObservations(columns, 2, 0)
	failed["repaired"] = Observation{Rows: checkRows(columns, 0), ErrorCode: "SQLITE_IOERR"}
	result := EvaluateCheckDefinition(*part.CheckDefinition, answer, failed)
	require.Equal(t, OutcomeUnknown, result.Outcome)
	require.Equal(t, StatusError, result.ExecutionStatus)
	require.Equal(t, ProblemExecutionError, result.Problem)

	repaired := checkCaseResultFor(t, result, "repaired")
	require.Equal(t, OutcomeUnknown, repaired.Outcome)
	require.Equal(t, ProblemExecutionError, repaired.Problem)
	require.Empty(t, repaired.ObservedOutcome)
	require.Contains(t, repaired.Message, "SQLITE_IOERR")

	// The same ordering applies to a malformed recordset: the execution error
	// is the reported problem, not a row-shape or case-outcome mismatch.
	failed["repaired"] = Observation{Rows: &apicontract.Recordset{}, ErrorCode: "SQLITE_CORRUPT"}
	result = EvaluateCheckDefinition(*part.CheckDefinition, answer, failed)
	require.Equal(t, OutcomeUnknown, result.Outcome)
	require.Equal(t, ProblemExecutionError, checkCaseResultFor(t, result, "repaired").Problem)
}

func checkCaseResultFor(t *testing.T, result CheckDefinitionResult, fixture string) CheckCaseResult {
	t.Helper()
	for _, item := range result.Cases {
		if item.Fixture == fixture {
			return item
		}
	}
	t.Fatalf("check case result %q not found", fixture)
	return CheckCaseResult{}
}

func TestCheckDefinitionManifestRejectsUnsupportedExpectation(t *testing.T) {
	part := checkPart()
	part.CheckDefinition = &CheckDefinitionPart{
		QueryIDs: []string{"queries/duplicate-emails"},
		Expectation: detective.Validator{
			ID: "unique-email", Scope: "set", Kind: "unique", Severity: "critical", Field: "email",
		},
		Cases: []CheckCase{{Fixture: "broken", ExpectedOutcome: OutcomeFail}},
	}
	require.Error(t, part.Validate())

	part = checkPart()
	part.CheckDefinition.QueryIDs = []string{"../escape"}
	require.Error(t, part.Validate())

	part = checkPart()
	part.CheckDefinition.Cases = []CheckCase{{Fixture: "broken", ExpectedOutcome: "unknown"}}
	require.Error(t, part.Validate())
}

func TestGradeAttemptRequiresEveryRequiredPart(t *testing.T) {
	opened := openGolden(t, "retry-evidence")
	manifest := opened.Manifest
	sqlPart := findPart(t, manifest, "requests")
	structuredPart := findPart(t, manifest, "evidence-pairs")

	sqlPass := SQLPartResult{Outcome: OutcomePass, ExecutionStatus: StatusCompleted}
	structuredPass := StructuredResult{Outcome: OutcomePass, ExecutionStatus: StatusCompleted}

	attempt, err := GradeAttempt(manifest, []PartOutcome{
		sqlPass.AsOutcome(sqlPart),
		structuredPass.AsOutcome(structuredPart),
	})
	require.NoError(t, err)
	require.True(t, attempt.Complete)
	require.Empty(t, attempt.BlockingParts)

	structuredInvalid := StructuredResult{
		Outcome: OutcomeUnknown, ExecutionStatus: StatusError, Problem: ProblemUnknownField,
		Message: "unknown field \"free-text\"",
	}
	attempt, err = GradeAttempt(manifest, []PartOutcome{
		sqlPass.AsOutcome(sqlPart),
		structuredInvalid.AsOutcome(structuredPart),
	})
	require.NoError(t, err)
	require.False(t, attempt.Complete)
	require.Equal(t, []string{"evidence-pairs"}, attempt.BlockingParts)

	// A failing optional part never grants or blocks completion.
	optionalManifest := manifest
	optionalManifest.Parts = append([]Part(nil), manifest.Parts...)
	for index := range optionalManifest.Parts {
		if optionalManifest.Parts[index].ID == structuredPart.ID {
			optionalManifest.Parts[index].Required = false
		}
	}
	attempt, err = GradeAttempt(optionalManifest, []PartOutcome{
		sqlPass.AsOutcome(sqlPart),
		{PartID: structuredPart.ID, AnswerKind: structuredPart.AnswerKind, Outcome: OutcomeFail, ExecutionStatus: StatusCompleted},
	})
	require.NoError(t, err)
	require.True(t, attempt.Complete)

	// Outcomes are matched to the definition, not trusted by position.
	_, err = GradeAttempt(manifest, []PartOutcome{sqlPass.AsOutcome(sqlPart)})
	require.Error(t, err)

	_, err = GradeAttempt(manifest, []PartOutcome{
		sqlPass.AsOutcome(sqlPart),
		sqlPass.AsOutcome(structuredPart),
		structuredPass.AsOutcome(structuredPart),
	})
	require.Error(t, err)

	missingPartOutcome := structuredPass.AsOutcome(structuredPart)
	missingPartOutcome.PartID = "missing"
	_, err = GradeAttempt(manifest, []PartOutcome{sqlPass.AsOutcome(sqlPart), missingPartOutcome})
	require.Error(t, err)

	spoofedKind := structuredPass.AsOutcome(structuredPart)
	spoofedKind.AnswerKind = AnswerSQL
	_, err = GradeAttempt(manifest, []PartOutcome{sqlPass.AsOutcome(sqlPart), spoofedKind})
	require.Error(t, err)
}

func TestFinalStateRejectsUndeclaredTablesAndIdentifiesInvariant(t *testing.T) {
	columns := []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "email", Type: "string"}}
	oracle := FinalStateOracle{
		Tables: []TableExpectation{{
			Name:    "customers",
			Columns: columns,
			Rows:    [][]apicontract.TypedValue{{integer("1"), text("a@x.com")}},
		}},
		Invariants: []Invariant{{
			ID: "keep-lowest-id", Kind: InvariantGroupMin, Table: "customers",
			GroupBy: []string{"email"}, Column: "id",
			Description: "Keep the lowest id per exact email value.",
		}},
	}
	require.NoError(t, oracle.Validate())

	actual := []TableState{{
		Name:    "customers",
		Columns: columns,
		Rows:    [][]apicontract.TypedValue{{integer("2"), text("a@x.com")}},
	}}
	result := EvaluateFinalState(oracle, actual)
	require.False(t, result.Pass)
	require.Equal(t, "keep-lowest-id", result.ViolatedInvariant())
	require.Equal(t, ProblemInvariant, result.Problem)

	actual = append(actual, TableState{Name: "extra", Columns: columns, Rows: [][]apicontract.TypedValue{{integer("3"), text("b@x.com")}}})
	result = EvaluateFinalState(oracle, actual)
	require.False(t, result.Pass)
	require.Contains(t, result.Tables, TableResult{Table: "extra", Problem: ProblemRowContent, Message: "observed table is not declared by the final-state oracle"})
}

func TestFinalStateShortObservedRowsFailInsteadOfPanickingOrPassing(t *testing.T) {
	columns := []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "email", Type: "string"}}
	oracle := FinalStateOracle{
		Tables: []TableExpectation{{
			Name:    "customers",
			Columns: columns,
			Rows:    [][]apicontract.TypedValue{{integer("1"), text("a@x.com")}},
		}},
		Invariants: []Invariant{{
			ID: "keep-lowest-id", Kind: InvariantGroupMin, Table: "customers",
			GroupBy: []string{"email"}, Column: "id",
			Description: "Keep the lowest id per exact email value.",
		}},
	}
	// The oracle itself is valid: the invariant is genuine and satisfiable.
	require.NoError(t, oracle.Validate())

	for name, rows := range map[string][][]apicontract.TypedValue{
		"empty row":              {{}},
		"group column truncated": {{integer("1")}},
		"retention only":         {{text("a@x.com")}},
		"row wider than columns": {{integer("1"), text("a@x.com"), text("extra")}},
	} {
		t.Run(name, func(t *testing.T) {
			result := EvaluateFinalState(oracle, []TableState{{Name: "customers", Columns: columns, Rows: rows}})
			require.False(t, result.Pass, "a malformed observed table must never pass")
			require.Equal(t, ProblemRowShape, result.Problem)
			require.Len(t, result.Tables, 1)
			require.False(t, result.Tables[0].Pass)
			require.Equal(t, ProblemRowShape, result.Tables[0].Problem)
			require.Len(t, result.Invariants, 1)
			require.False(t, result.Invariants[0].Pass, "invariants must not be evaluated on a malformed row")
			require.Equal(t, ProblemRowShape, result.Invariants[0].Problem)
		})
	}

	// A malformed expected table is refused the same way instead of panicking
	// when the oracle is used without Validate first.
	malformedOracle := oracle
	malformedOracle.Tables = []TableExpectation{{
		Name: "customers", Columns: columns,
		Rows: [][]apicontract.TypedValue{{integer("1")}},
	}}
	result := EvaluateFinalState(malformedOracle, []TableState{{Name: "customers", Columns: columns, Rows: [][]apicontract.TypedValue{{integer("1"), text("a@x.com")}}}})
	require.False(t, result.Pass)
	require.Equal(t, ProblemRowShape, result.Problem)
	require.Equal(t, ProblemRowShape, result.Invariants[0].Problem)

	// The same malformed observation fails SQL grading as a recoverable
	// failure rather than panicking inside the final-state oracle.
	sqlPart := SQLPart{Cases: []SQLCase{{
		Fixture: "customers",
		Oracle:  Oracle{Kind: OracleFinalState, FinalState: &oracle},
	}}}
	partResult := GradeSQLPart(sqlPart, "DELETE FROM customers;", map[string]Observation{
		"customers": {Tables: []TableState{{
			Name: "customers", Columns: columns,
			Rows: [][]apicontract.TypedValue{{integer("1")}},
		}}},
	})
	require.Equal(t, OutcomeFail, partResult.Outcome)
	require.Equal(t, StatusCompleted, partResult.ExecutionStatus)
	require.Equal(t, ProblemRowShape, partResult.Problem)
}

func TestGoldenRetryEvidenceGradesTypedResultAndStructuredParts(t *testing.T) {
	opened := openGolden(t, "retry-evidence")
	manifest := opened.Manifest
	sqlPart := findPart(t, manifest, "requests")
	structuredPart := findPart(t, manifest, "evidence-pairs")
	answers := loadGoldenAnswers(t, "retry-evidence")

	reference := answerByID(t, answers, "reference")
	alternative := answerByID(t, answers, "alternative")
	wrong := answerByID(t, answers, "wrong-same-row-count")

	referenceResult := GradeSQLPart(*sqlPart.SQL, reference.SQL, observationsOf(t, reference))
	alternativeResult := GradeSQLPart(*sqlPart.SQL, alternative.SQL, observationsOf(t, alternative))
	wrongResult := GradeSQLPart(*sqlPart.SQL, wrong.SQL, observationsOf(t, wrong))

	require.Equal(t, OutcomePass, referenceResult.Outcome, referenceResult.Message)
	require.Equal(t, OutcomePass, alternativeResult.Outcome, alternativeResult.Message)
	require.Equal(t, OutcomeFail, wrongResult.Outcome)

	// The wrong query returns the expected row count with the wrong contents.
	expectedRows := findSQLCase(t, *sqlPart.SQL, "retry-evidence").Oracle.Result.Rows
	actualRows := observationsOf(t, wrong)["retry-evidence"].Rows
	require.NotNil(t, actualRows)
	require.Len(t, actualRows.Rows, len(expectedRows))
	require.Equal(t, ProblemRowContent, wrongResult.Cases[0].Problem)

	correctSelections := StructuredAnswer{Selections: map[string][]string{
		"evidence-pairs": {"r2-a2-a3-trace", "r4-a6-a7-trace"},
	}}
	correct := EvaluateStructured(*structuredPart.Structured, correctSelections)
	require.Equal(t, OutcomePass, correct.Outcome)

	sqlOutcome := referenceResult.AsOutcome(sqlPart)
	attempt, err := GradeAttempt(manifest, []PartOutcome{sqlOutcome, correct.AsOutcome(structuredPart)})
	require.NoError(t, err)
	require.True(t, attempt.Complete)

	wrongEvidence := StructuredAnswer{Selections: map[string][]string{
		"evidence-pairs": {"r2-a2-a3-trace", "r3-a4-a5-trace"},
	}}
	wrongEvidenceResult := EvaluateStructured(*structuredPart.Structured, wrongEvidence)
	attempt, err = GradeAttempt(manifest, []PartOutcome{sqlOutcome, wrongEvidenceResult.AsOutcome(structuredPart)})
	require.NoError(t, err)
	require.False(t, attempt.Complete)
	require.Equal(t, []string{"evidence-pairs"}, attempt.BlockingParts)

	unknownField := StructuredAnswer{Selections: map[string][]string{
		"evidence-pairs": {"r2-a2-a3-trace", "r4-a6-a7-trace"},
		"free-text":      {"anything"},
	}}
	unknownFieldResult := EvaluateStructured(*structuredPart.Structured, unknownField)
	require.Equal(t, StatusError, unknownFieldResult.ExecutionStatus)
	require.Equal(t, ProblemUnknownField, unknownFieldResult.Problem)
	attempt, err = GradeAttempt(manifest, []PartOutcome{sqlOutcome, unknownFieldResult.AsOutcome(structuredPart)})
	require.NoError(t, err)
	require.False(t, attempt.Complete)
	require.Equal(t, []string{"evidence-pairs"}, attempt.BlockingParts)
}

func TestGoldenDeduplicationSuitePreservesLowestIDAndRejectsWrongAnswers(t *testing.T) {
	opened := openGolden(t, "deduplicate-customers")
	manifest := opened.Manifest
	sqlPart := findPart(t, manifest, "delete-duplicates")
	answers := loadGoldenAnswers(t, "deduplicate-customers")

	for _, id := range []string{"reference", "alternative"} {
		answer := answerByID(t, answers, id)
		result := GradeSQLPart(*sqlPart.SQL, answer.SQL, observationsOf(t, answer))
		require.Equal(t, OutcomePass, result.Outcome, "%s: %s", id, result.Message)
		for _, observation := range answer.Fixtures {
			require.Equalf(t, observation.OnceTables, observation.Tables, "%s must be idempotent on %s", id, observation.Fixture)
		}
	}

	nonsequentialCase := findSQLCase(t, *sqlPart.SQL, "nonsequential")
	expectedRows := nonsequentialCase.Oracle.FinalState.Tables[0].Rows

	createdAt := answerByID(t, answers, "wrong-earliest-created-at")
	result := GradeSQLPart(*sqlPart.SQL, createdAt.SQL, observationsOf(t, createdAt))
	require.Equal(t, OutcomeFail, result.Outcome)
	nonsequential := caseResultFor(t, result, "nonsequential")
	require.Equal(t, ProblemInvariant, nonsequential.Problem)
	require.Equal(t, "keep-lowest-id", nonsequential.InvariantID)
	require.Contains(t, nonsequential.Message, "keep-lowest-id")
	actualRows := observationsOf(t, createdAt)["nonsequential"].Tables[0].Rows
	require.Len(t, actualRows, len(expectedRows), "the wrong answer has the expected row count but the wrong contents")
	require.NotEqual(t, expectedRows, actualRows)
	require.Contains(t, rowIDs(t, expectedRows), "2")
	require.Contains(t, rowIDs(t, actualRows), "5", "the wrong answer keeps the earliest created_at instead of the lowest id")
	require.Contains(t, rowIDs(t, expectedRows), "20")
	require.Contains(t, rowIDs(t, actualRows), "21", "NULL is one group and its lowest id must survive")

	maxID := answerByID(t, answers, "wrong-max-id")
	require.Equal(t, OutcomeFail, GradeSQLPart(*sqlPart.SQL, maxID.SQL, observationsOf(t, maxID)).Outcome)

	// BINARY equality: a NOCASE grouping deletes distinct case variants.
	noCase := answerByID(t, answers, "wrong-nocase")
	result = GradeSQLPart(*sqlPart.SQL, noCase.SQL, observationsOf(t, noCase))
	require.Equal(t, OutcomeFail, result.Outcome)
	require.Equal(t, OutcomeFail, caseResultFor(t, result, "null-text").Outcome)
	nullTextExpected := findSQLCase(t, *sqlPart.SQL, "null-text").Oracle.FinalState.Tables[0].Rows
	nullTextActual := observationsOf(t, noCase)["null-text"].Tables[0].Rows
	require.Contains(t, rowIDs(t, nullTextExpected), "6")
	require.NotContains(t, rowIDs(t, nullTextActual), "6", "BINARY equality keeps A@x.com and a@x.com apart")

	// A hardcoded answer that deletes the right number of rows must still fail.
	hardcoded := answerByID(t, answers, "wrong-hardcoded")
	result = GradeSQLPart(*sqlPart.SQL, hardcoded.SQL, observationsOf(t, hardcoded))
	require.Equal(t, OutcomeFail, result.Outcome)
	hardcodedRows := observationsOf(t, hardcoded)["nonsequential"].Tables[0].Rows
	require.Len(t, hardcodedRows, len(expectedRows))

	// Every broken, repaired and mutated fixture is represented.
	check := findPart(t, manifest, "prevention-check")
	var fixtures []string
	for _, item := range check.CheckDefinition.Cases {
		fixtures = append(fixtures, item.Fixture)
	}
	require.Contains(t, fixtures, "nonsequential")
	require.Contains(t, fixtures, "repaired")
	require.Contains(t, fixtures, "repaired-mutated")

	checkAnswer := CheckDefinitionAnswer{QueryID: check.CheckDefinition.QueryIDs[0], Expectation: check.CheckDefinition.Expectation}
	checkObservations := observationsOf(t, answerByID(t, answers, "check-duplicate-groups"))
	resultCheck := EvaluateCheckDefinition(*check.CheckDefinition, checkAnswer, checkObservations)
	require.Equal(t, OutcomePass, resultCheck.Outcome, resultCheck.Message)

	attempt, err := GradeAttempt(manifest, []PartOutcome{
		GradeSQLPart(*sqlPart.SQL, answerByID(t, answers, "reference").SQL, observationsOf(t, answerByID(t, answers, "reference"))).AsOutcome(sqlPart),
		resultCheck.AsOutcome(check),
	})
	require.NoError(t, err)
	require.True(t, attempt.Complete)
}

func rowIDs(t *testing.T, rows [][]apicontract.TypedValue) []string {
	t.Helper()
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		require.NotEmpty(t, row)
		ids = append(ids, row[0].Str)
	}
	return ids
}

func loadGoldenAnswers(t *testing.T, challengeID string) []goldenAnswer {
	t.Helper()
	var golden goldenFile
	data := readTestFile(t, "testdata/goldens/"+challengeID+".json")
	require.NoError(t, apicontract.DecodeStrict(data, &golden))
	require.Equal(t, challengeID, golden.Challenge)
	require.NotEmpty(t, golden.GeneratedWith["sqlite"])
	require.NotEmpty(t, golden.GeneratedWith["python"])
	opened := openGolden(t, challengeID)
	for _, answer := range golden.Answers {
		require.Contains(t, []string{"sql", "check-observation"}, answer.Kind)
		for _, observation := range answer.Fixtures {
			var fixture Fixture
			for _, candidate := range opened.Manifest.FixtureSuite.Fixtures {
				if candidate.ID == observation.Fixture {
					fixture = candidate
				}
			}
			require.NotEmpty(t, fixture.ID)
			require.Equal(t, fixture.SHA256, observation.FixtureSHA256, "golden observation must identify the pristine fixture bytes it came from")
			if answer.Kind == "sql" {
				require.Equal(t, fixture.Runs, observation.Runs, "graded SQL observations use the manifest's run count")
			}
		}
	}
	return golden.Answers
}

func answerByID(t *testing.T, answers []goldenAnswer, id string) goldenAnswer {
	t.Helper()
	for _, answer := range answers {
		if answer.ID == id {
			return answer
		}
	}
	t.Fatalf("golden answer %q not found", id)
	return goldenAnswer{}
}

func observationsOf(t *testing.T, answer goldenAnswer) map[string]Observation {
	t.Helper()
	observations := make(map[string]Observation, len(answer.Fixtures))
	for _, fixture := range answer.Fixtures {
		observation := Observation{Tables: fixture.Tables, Rows: fixture.Rows}
		observations[fixture.Fixture] = observation
	}
	return observations
}

func caseResultFor(t *testing.T, result SQLPartResult, fixture string) SQLCaseResult {
	t.Helper()
	for _, item := range result.Cases {
		if item.Fixture == fixture {
			return item
		}
	}
	t.Fatalf("case result %q not found", fixture)
	return SQLCaseResult{}
}

func TestGoldenIsStrictJSONForTheBrowserContract(t *testing.T) {
	for _, challengeID := range []string{"deduplicate-customers", "retry-evidence"} {
		data := goldenManifestData(t, challengeID)
		var raw any
		require.NoError(t, json.Unmarshal(data, &raw))
		var manifest Manifest
		require.NoError(t, apicontract.DecodeStrict(data, &manifest))
		require.Equal(t, challengeID, manifest.ChallengeID)
	}
}
