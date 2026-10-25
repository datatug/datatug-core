package challenge

import (
	"fmt"
	"sort"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

// Outcome and execution-status vocabulary shared with Detective results.
const (
	OutcomePass    = "pass"
	OutcomeFail    = "fail"
	OutcomeUnknown = "unknown"

	StatusCompleted = "completed"
	StatusError     = "error"
)

// SQL grading problem codes.
const (
	ProblemSQLBytes       = "sql-bytes"
	ProblemSQLEmpty       = "sql-empty"
	ProblemExecutionError = "execution-error"
	ProblemObservation    = "observation-missing"
	ProblemOracleInvalid  = "invalid-oracle"
	ProblemFinalState     = "final-state"
)

// Observation is one pristine-fixture run supplied by the disposable runtime.
// It is not learner input and carries no authorization claim.
type Observation struct {
	Rows      *apicontract.Recordset `json:"rows,omitempty"`
	Tables    []TableState           `json:"tables,omitempty"`
	ErrorCode string                 `json:"errorCode,omitempty"`
}

// SQLCaseResult reports one fixture's graded outcome. Missing, Unexpected and
// Examples are present only for a same-shape unordered row-content mismatch;
// they carry bounded, deterministic feedback and never affect the verdict.
type SQLCaseResult struct {
	Fixture         string   `json:"fixture"`
	Outcome         string   `json:"outcome"`
	ExecutionStatus string   `json:"executionStatus"`
	Problem         string   `json:"problem,omitempty"`
	InvariantID     string   `json:"invariantId,omitempty"`
	Message         string   `json:"message,omitempty"`
	Missing         int      `json:"missing,omitempty"`
	Unexpected      int      `json:"unexpected,omitempty"`
	Examples        []string `json:"examples,omitempty"`
}

// SQLPartResult reports every fixture case for one SQL answer.
type SQLPartResult struct {
	Outcome         string          `json:"outcome"`
	ExecutionStatus string          `json:"executionStatus"`
	Problem         string          `json:"problem,omitempty"`
	Message         string          `json:"message,omitempty"`
	Cases           []SQLCaseResult `json:"cases"`
}

// AsOutcome projects the result onto the attempt contract.
func (r SQLPartResult) AsOutcome(part Part) PartOutcome {
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

// GradeSQLPart grades one SQL text against every declared case. The runtime
// executes it on independent, pristine fixture copies; this function only
// compares typed evidence.
func GradeSQLPart(part SQLPart, sql string, observations map[string]Observation) SQLPartResult {
	result := SQLPartResult{Outcome: OutcomePass, ExecutionStatus: StatusCompleted}
	if len(sql) == 0 {
		return SQLPartResult{Outcome: OutcomeUnknown, ExecutionStatus: StatusError, Problem: ProblemSQLEmpty, Message: "SQL answer is empty"}
	}
	if len(sql) > MaxSQLBytes {
		return SQLPartResult{Outcome: OutcomeUnknown, ExecutionStatus: StatusError, Problem: ProblemSQLBytes, Message: fmt.Sprintf("SQL answer exceeds %d bytes", MaxSQLBytes)}
	}
	for _, item := range part.Cases {
		caseResult := SQLCaseResult{Fixture: item.Fixture, Outcome: OutcomePass, ExecutionStatus: StatusCompleted}
		observation, ok := observations[item.Fixture]
		switch {
		case !ok:
			caseResult.Outcome = OutcomeUnknown
			caseResult.ExecutionStatus = StatusError
			caseResult.Problem = ProblemObservation
			caseResult.Message = fmt.Sprintf("no observation for fixture %q", item.Fixture)
		case observation.ErrorCode != "":
			caseResult.Outcome = OutcomeUnknown
			caseResult.ExecutionStatus = StatusError
			caseResult.Problem = ProblemExecutionError
			caseResult.Message = fmt.Sprintf("fixture %q execution failed with %q", item.Fixture, observation.ErrorCode)
		default:
			if err := item.Oracle.Validate(); err != nil {
				caseResult.Outcome = OutcomeUnknown
				caseResult.ExecutionStatus = StatusError
				caseResult.Problem = ProblemOracleInvalid
				caseResult.Message = err.Error()
				break
			}
			switch item.Oracle.Kind {
			case OracleResult:
				if observation.Rows == nil {
					caseResult.Outcome = OutcomeUnknown
					caseResult.ExecutionStatus = StatusError
					caseResult.Problem = ProblemObservation
					caseResult.Message = fmt.Sprintf("fixture %q returned no recordset", item.Fixture)
					break
				}
				diff := CompareTypedRows(item.Oracle.Result.Columns, observation.Rows.Columns, item.Oracle.Result.Rows, observation.Rows.Rows, RowOptions{
					Ordering:           item.Oracle.Result.Ordering,
					RequireColumnNames: item.Oracle.Result.RequireColumnNames,
					NumericTolerance:   item.Oracle.Result.NumericTolerance,
				})
				if !diff.Match {
					caseResult.Outcome = OutcomeFail
					caseResult.Problem = diff.Problem
					caseResult.Message = diff.Message
					// Row-content feedback is bounded and copied so callers can
					// retain or mutate one result without affecting another.
					caseResult.Missing = diff.Missing
					caseResult.Unexpected = diff.Unexpected
					caseResult.Examples = boundedExamples(diff.Examples)
				}
			case OracleFinalState:
				if len(observation.Tables) == 0 {
					caseResult.Outcome = OutcomeUnknown
					caseResult.ExecutionStatus = StatusError
					caseResult.Problem = ProblemObservation
					caseResult.Message = fmt.Sprintf("fixture %q returned no final-state tables", item.Fixture)
					break
				}
				final := EvaluateFinalState(*item.Oracle.FinalState, observation.Tables)
				if !final.Pass {
					caseResult.Outcome = OutcomeFail
					caseResult.Problem = ProblemFinalState
					// A malformed observed row is a structural failure; it must
					// not be reported as a violated retention invariant.
					if final.Problem == ProblemRowShape {
						caseResult.Problem = ProblemRowShape
					}
					if final.Problem == ProblemInvariant {
						if invariant := final.ViolatedInvariant(); invariant != "" {
							caseResult.Problem = ProblemInvariant
							caseResult.InvariantID = invariant
						}
					}
					caseResult.Message = final.Message
				}
			}
		}
		result.Cases = append(result.Cases, caseResult)
		result.absorb(caseResult)
	}
	return result
}

func (r *SQLPartResult) absorb(item SQLCaseResult) {
	if item.Outcome == OutcomeFail && r.Outcome != OutcomeFail {
		r.Outcome = OutcomeFail
	}
	if item.Outcome == OutcomeUnknown && r.Outcome == OutcomePass {
		r.Outcome = OutcomeUnknown
	}
	if item.ExecutionStatus == StatusError {
		r.ExecutionStatus = StatusError
	}
	if item.Outcome != OutcomePass || item.ExecutionStatus != StatusCompleted {
		if r.Problem == "" {
			r.Problem = item.Problem
			r.Message = fmt.Sprintf("fixture %q: %s", item.Fixture, item.Message)
		}
	}
}

// PartOutcome is the normalized, untrusted-input-resistant result of one part.
type PartOutcome struct {
	PartID          string `json:"partId"`
	AnswerKind      string `json:"answerKind"`
	Required        bool   `json:"required"`
	Outcome         string `json:"outcome"`
	ExecutionStatus string `json:"executionStatus"`
	Problem         string `json:"problem,omitempty"`
	Message         string `json:"message,omitempty"`
}

// AttemptOutcome is the only place completion is derived. It is true only
// when every required part passed with a completed execution.
type AttemptOutcome struct {
	Complete      bool          `json:"complete"`
	Parts         []PartOutcome `json:"parts"`
	BlockingParts []string      `json:"blockingParts,omitempty"`
}

// GradeAttempt validates that the provided outcomes correspond exactly to the
// manifest's parts and derives completion. It never accepts a caller-supplied
// completion boolean.
func GradeAttempt(manifest Manifest, outcomes []PartOutcome) (AttemptOutcome, error) {
	if err := manifest.Validate(); err != nil {
		return AttemptOutcome{}, err
	}
	parts := make(map[string]Part, len(manifest.Parts))
	for _, part := range manifest.Parts {
		parts[part.ID] = part
	}
	byID := make(map[string]PartOutcome, len(outcomes))
	for _, outcome := range outcomes {
		part, ok := parts[outcome.PartID]
		if !ok {
			return AttemptOutcome{}, refuse(CodeInvalidAnswer, "partId", "outcome names unknown part %q", outcome.PartID)
		}
		if _, exists := byID[outcome.PartID]; exists {
			return AttemptOutcome{}, refuse(CodeInvalidAnswer, "partId", "duplicate outcome for part %q", outcome.PartID)
		}
		if outcome.AnswerKind != part.AnswerKind {
			return AttemptOutcome{}, refuse(CodeInvalidAnswer, "answerKind", "outcome for part %q has answer kind %q, expected %q", outcome.PartID, outcome.AnswerKind, part.AnswerKind)
		}
		if !oneOf(outcome.Outcome, OutcomePass, OutcomeFail, OutcomeUnknown) {
			return AttemptOutcome{}, refuse(CodeInvalidAnswer, "outcome", "part %q has unsupported outcome %q", outcome.PartID, outcome.Outcome)
		}
		if !oneOf(outcome.ExecutionStatus, StatusCompleted, StatusError) {
			return AttemptOutcome{}, refuse(CodeInvalidAnswer, "executionStatus", "part %q has unsupported execution status %q", outcome.PartID, outcome.ExecutionStatus)
		}
		outcome.Required = part.Required
		byID[outcome.PartID] = outcome
	}
	if len(byID) != len(manifest.Parts) {
		var missing []string
		for _, part := range manifest.Parts {
			if _, ok := byID[part.ID]; !ok {
				missing = append(missing, part.ID)
			}
		}
		sort.Strings(missing)
		return AttemptOutcome{}, refuse(CodeInvalidAnswer, "parts", "missing outcomes for parts %v", missing)
	}
	result := AttemptOutcome{Complete: true, Parts: make([]PartOutcome, 0, len(manifest.Parts))}
	for _, part := range manifest.Parts {
		outcome := byID[part.ID]
		result.Parts = append(result.Parts, outcome)
		if part.Required && (outcome.Outcome != OutcomePass || outcome.ExecutionStatus != StatusCompleted) {
			result.Complete = false
			result.BlockingParts = append(result.BlockingParts, part.ID)
		}
	}
	return result, nil
}
