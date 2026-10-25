package challenge

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
)

// CheckDefinitionPart asks the learner to select one saved training query and
// one typed supported expectation. Outcomes are validated deterministically on
// broken, repaired and mutated fixtures; no model judgement is involved.
type CheckDefinitionPart struct {
	QueryIDs    []string            `json:"queryIds"`
	Expectation detective.Validator `json:"expectation"`
	Cases       []CheckCase         `json:"cases"`
}

// CheckCase pins one fixture to the outcome a correct Check must produce.
type CheckCase struct {
	Fixture         string `json:"fixture"`
	ExpectedOutcome string `json:"expectedOutcome"`
}

// CheckDefinitionAnswer is a learner's saved-query selection plus expectation.
type CheckDefinitionAnswer struct {
	QueryID     string              `json:"queryId"`
	Expectation detective.Validator `json:"expectation"`
}

// Stable check-definition problem codes.
const (
	ProblemUnknownQuery           = "unknown-query"
	ProblemUnsupportedExpectation = "unsupported-expectation"
	ProblemExpectationMismatch    = "expectation-mismatch"
	ProblemMissingObservation     = "missing-observation"
	ProblemCaseOutcome            = "case-outcome"
)

// CheckCaseResult reports one fixture's expected versus observed outcome.
type CheckCaseResult struct {
	Fixture         string `json:"fixture"`
	ExpectedOutcome string `json:"expectedOutcome"`
	ObservedOutcome string `json:"observedOutcome,omitempty"`
	Outcome         string `json:"outcome"`
	Problem         string `json:"problem,omitempty"`
	Message         string `json:"message,omitempty"`
}

// CheckDefinitionResult is the outcome of a Check-definition answer.
type CheckDefinitionResult struct {
	Outcome         string            `json:"outcome"`
	ExecutionStatus string            `json:"executionStatus"`
	Problem         string            `json:"problem,omitempty"`
	Message         string            `json:"message,omitempty"`
	Cases           []CheckCaseResult `json:"cases"`
}

// AsOutcome projects the result onto the attempt contract.
func (r CheckDefinitionResult) AsOutcome(part Part) PartOutcome {
	return PartOutcome{
		PartID:          part.ID,
		AnswerKind:      part.AnswerKind,
		Required:        part.Required,
		Outcome:         r.Outcome,
		ExecutionStatus: r.ExecutionStatus,
		Problem:         r.Problem,
		Message:         r.Message,
	}
}

func (c CheckDefinitionPart) validate(fixtureIDs map[string]bool) error {
	if len(c.QueryIDs) == 0 || len(c.QueryIDs) > maxQueryIDs {
		return refuse(CodeInvalidManifest, "checkDefinition.queryIds", "must contain 1..%d saved-query references", maxQueryIDs)
	}
	seenQueries := map[string]bool{}
	for _, queryID := range c.QueryIDs {
		if !validQueryRef(queryID) || seenQueries[queryID] {
			return refuse(CodeInvalidManifest, "checkDefinition.queryIds", "saved-query references must be valid and unique")
		}
		seenQueries[queryID] = true
	}
	if _, err := supportedExpectationCount(c.Expectation); err != nil {
		return refuse(CodeInvalidManifest, "checkDefinition.expectation", "%s", err)
	}
	if len(c.Cases) == 0 || len(c.Cases) > maxManifestCases {
		return refuse(CodeInvalidManifest, "checkDefinition.cases", "must contain 1..%d cases", maxManifestCases)
	}
	seenFixtures := map[string]bool{}
	for _, item := range c.Cases {
		if !validID(item.Fixture) || seenFixtures[item.Fixture] || !fixtureIDs[item.Fixture] {
			return refuse(CodeInvalidManifest, "checkDefinition.cases", "case fixtures must be valid, pinned and appear once")
		}
		seenFixtures[item.Fixture] = true
		if item.ExpectedOutcome != OutcomePass && item.ExpectedOutcome != OutcomeFail {
			return refuse(CodeInvalidManifest, "checkDefinition.cases", "expectedOutcome must be pass or fail")
		}
	}
	return nil
}

func supportedExpectationCount(expectation detective.Validator) (int, error) {
	if err := detective.ValidateValidator(expectation); err != nil {
		return 0, fmt.Errorf("expectation must be a valid shared Validator")
	}
	if expectation.Kind != "row-count" || expectation.Scope != "set" || expectation.Expected == nil ||
		expectation.Expected.Type != apicontract.ValueTypeInteger || expectation.ValidatorID != "" ||
		expectation.Field != "" || expectation.OtherField != "" || expectation.CheckID != "" || len(expectation.Bindings) != 0 || expectation.OnUnavailable != "" {
		return 0, fmt.Errorf("only a set-scoped row-count integer expectation is supported")
	}
	count, err := strconv.ParseInt(expectation.Expected.Str, 10, 64)
	if err != nil || count < 0 || count > MaxResultRows {
		return 0, fmt.Errorf("row-count expectation must be between 0 and %d", MaxResultRows)
	}
	return int(count), nil
}

func validateCheckDefinitionAnswer(part CheckDefinitionPart, answer CheckDefinitionAnswer) error {
	if !oneOf(answer.QueryID, part.QueryIDs...) {
		return fmt.Errorf("query selection must be one of the declared saved queries")
	}
	if _, err := supportedExpectationCount(answer.Expectation); err != nil {
		return err
	}
	if !sameExpectationSemantics(part.Expectation, answer.Expectation) {
		return fmt.Errorf("expectation must equal the declared expected expectation")
	}
	return nil
}

func sameExpectationSemantics(left, right detective.Validator) bool {
	type semantics struct {
		Scope         string
		Kind          string
		Field         string
		OtherField    string
		Expected      *apicontract.TypedValue
		CheckID       string
		Bindings      []detective.Binding
		OnUnavailable string
	}
	canonical := func(value detective.Validator) (string, error) {
		data, err := json.Marshal(semantics{
			Scope: value.Scope, Kind: value.Kind, Field: value.Field, OtherField: value.OtherField,
			Expected: value.Expected, CheckID: value.CheckID, Bindings: value.Bindings, OnUnavailable: value.OnUnavailable,
		})
		return string(data), err
	}
	leftKey, leftErr := canonical(left)
	rightKey, rightErr := canonical(right)
	return leftErr == nil && rightErr == nil && leftKey == rightKey
}

// EvaluateCheckDefinition grades a submitted Check definition. Observation
// rows must come from the disposable runtime executing the selected saved
// query against each pristine fixture.
func EvaluateCheckDefinition(part CheckDefinitionPart, answer CheckDefinitionAnswer, observations map[string]Observation) CheckDefinitionResult {
	if !oneOf(answer.QueryID, part.QueryIDs...) {
		return CheckDefinitionResult{
			Outcome: OutcomeUnknown, ExecutionStatus: StatusError, Problem: ProblemUnknownQuery,
			Message: fmt.Sprintf("query %q is not one of the declared saved queries", answer.QueryID),
		}
	}
	if _, err := supportedExpectationCount(answer.Expectation); err != nil {
		return CheckDefinitionResult{
			Outcome: OutcomeUnknown, ExecutionStatus: StatusError, Problem: ProblemUnsupportedExpectation,
			Message: err.Error(),
		}
	}
	if !sameExpectationSemantics(part.Expectation, answer.Expectation) {
		return CheckDefinitionResult{
			Outcome: OutcomeFail, ExecutionStatus: StatusCompleted, Problem: ProblemExpectationMismatch,
			Message: "the submitted expectation does not equal the declared expected expectation",
		}
	}
	count, _ := supportedExpectationCount(part.Expectation)

	result := CheckDefinitionResult{Outcome: OutcomePass, ExecutionStatus: StatusCompleted}
	for _, item := range part.Cases {
		observation, ok := observations[item.Fixture]
		caseResult := gradeCheckCase(item, observation, ok, count)
		result.Cases = append(result.Cases, caseResult)
		switch caseResult.Outcome {
		case OutcomeUnknown:
			result.ExecutionStatus = StatusError
			result.Outcome = OutcomeUnknown
			if result.Problem == "" {
				result.Problem = caseResult.Problem
				result.Message = caseResult.Message
			}
		case OutcomeFail:
			if result.Outcome == OutcomePass {
				result.Outcome = OutcomeFail
				result.Problem = caseResult.Problem
				result.Message = caseResult.Message
			}
		}
	}
	return result
}

// validateObservationRecordset applies the challenge-local result rules to a
// Check-definition observation. It deliberately does not call the shared
// apicontract Recordset validator: grading accepts heterogeneous SQLite cells
// and all-NULL columns, while still refusing unsupported column metadata,
// malformed typed cells and a wrong row width.
func validateObservationRecordset(recordset *apicontract.Recordset) *RowDiff {
	if diff := validateObservedColumns(recordset.Columns); diff != nil {
		return diff
	}
	return validateRowTypes(recordset.Columns, recordset.Rows)
}

func unavailableCheckCase(item CheckCase, problem, message string) CheckCaseResult {
	return CheckCaseResult{
		Fixture: item.Fixture, ExpectedOutcome: item.ExpectedOutcome,
		Outcome: OutcomeUnknown, Problem: problem, Message: message,
	}
}

// gradeCheckCase grades one pristine-fixture observation. A runtime execution
// error is checked before rows: a partially populated or coincidentally
// matching recordset from a failed run must never be graded as a pass.
func gradeCheckCase(item CheckCase, observation Observation, ok bool, expectedCount int) CheckCaseResult {
	if !ok {
		return unavailableCheckCase(item, ProblemMissingObservation, fmt.Sprintf("fixture %q has no observed result", item.Fixture))
	}
	if observation.ErrorCode != "" {
		return unavailableCheckCase(item, ProblemExecutionError, fmt.Sprintf("fixture %q execution failed with %q", item.Fixture, observation.ErrorCode))
	}
	if observation.Rows == nil {
		return unavailableCheckCase(item, ProblemMissingObservation, fmt.Sprintf("fixture %q returned no result rows", item.Fixture))
	}
	if diff := validateObservationRecordset(observation.Rows); diff != nil {
		return unavailableCheckCase(item, ProblemMissingObservation, fmt.Sprintf("fixture %q returned malformed rows: %s", item.Fixture, diff.Message))
	}
	observed := OutcomePass
	if len(observation.Rows.Rows) != expectedCount {
		observed = OutcomeFail
	}
	caseResult := CheckCaseResult{Fixture: item.Fixture, ExpectedOutcome: item.ExpectedOutcome, ObservedOutcome: observed, Outcome: OutcomePass}
	if observed != item.ExpectedOutcome {
		caseResult.Outcome = OutcomeFail
		caseResult.Problem = ProblemCaseOutcome
		caseResult.Message = fmt.Sprintf("fixture %q expected %s, observed %s", item.Fixture, item.ExpectedOutcome, observed)
	}
	return caseResult
}
