package detective

import (
	"context"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

// RunRequest contains server-issued identity and names only. Input values are
// used during evaluation and never copied into CheckRun or the audit stream.
type RunRequest struct {
	ID          string
	CheckID     string
	QueryID     string
	CaseID      string
	Environment string
	Actor       string
	Inputs      map[string]InputValue
}

// EvaluateCheck runs a preloaded, validated transitive Catalog. The caller
// authorizes project/case access and supplies the secured saved-query runner.
// It persists the returned run and case link in its own transaction.
func EvaluateCheck(ctx context.Context, req RunRequest, catalog Catalog, runner QueryRunner, metadata ExecutionMetadata) (CheckRun, error) {
	if !catalog.sealed() || !validSlug(req.ID) || !validSlug(req.CheckID) || req.QueryID != "" || req.Environment == "" || runner == nil {
		return CheckRun{}, ErrInvalid
	}
	if err := ValidateCatalog(catalog); err != nil {
		return CheckRun{}, err
	}
	root, ok := catalog.Checks[req.CheckID]
	if !ok {
		return CheckRun{}, ErrNotFound
	}
	if err := validateInputs(root.Inputs, req.Inputs); err != nil {
		return CheckRun{}, err
	}
	start := time.Now().UTC()
	run := CheckRun{ID: req.ID, Kind: "check", CheckID: req.CheckID, Definition: root, CaseID: req.CaseID, Environment: req.Environment, InputNames: sortedKeys(req.Inputs), StartedAt: start, Actor: req.Actor}
	e := newEngine(catalog, req.Environment, runner, metadata)
	e.snapshot(&run, req.CheckID, "")
	runCtx, cancel := context.WithTimeout(ctx, maxRunDuration)
	defer cancel()
	result := e.runCheck(runCtx, req.CheckID, req.Inputs, 0, map[string]bool{})
	run.Steps = result.Children
	run.ExecutionStatus, run.Outcome, run.Severity = result.ExecutionStatus, result.Outcome, result.Severity
	run.DurationMs = time.Since(start).Milliseconds()
	run.CheckInvocations, run.QueryExecutions = e.nodes, e.queries
	return run, nil
}

// EvaluateQueryValidators runs a saved query and its local Validators without
// creating a synthetic global Check. Catalog must contain that local entry.
func EvaluateQueryValidators(ctx context.Context, req RunRequest, catalog Catalog, runner QueryRunner, metadata ExecutionMetadata) (CheckRun, error) {
	if !catalog.sealed() || !validSlug(req.ID) || !validQueryRef(req.QueryID) || req.CheckID != "" || req.Environment == "" || runner == nil {
		return CheckRun{}, ErrInvalid
	}
	if err := ValidateCatalog(catalog); err != nil {
		return CheckRun{}, err
	}
	local, found := catalog.QueryValidators[req.QueryID]
	if !found {
		return CheckRun{}, ErrNotFound
	}
	bindings := make([]Binding, 0, len(req.Inputs))
	for name, value := range req.Inputs {
		if !validParameterName(name) || value.Range != nil || value.Validate() != nil {
			return CheckRun{}, ErrInvalid
		}
		bindings = append(bindings, Binding{Input: name, FromInput: name})
	}
	e := newEngine(catalog, req.Environment, runner, metadata)
	for _, validator := range local.Validators {
		v := validator
		if v.ValidatorID != "" {
			v = e.validators[v.ValidatorID].Validator
		}
		if v.Kind == "check" {
			for _, binding := range v.Bindings {
				if binding.FromInput != "" {
					if _, ok := req.Inputs[binding.FromInput]; !ok {
						return CheckRun{}, ErrInvalid
					}
				}
			}
		}
	}
	start := time.Now().UTC()
	runCtx, cancel := context.WithTimeout(ctx, maxRunDuration)
	defer cancel()
	step := e.runQueryStep(runCtx, Step{ID: "query", Kind: "query", QueryID: req.QueryID, Bindings: bindings}, req.Inputs, 0, map[string]bool{})
	run := CheckRun{ID: req.ID, Kind: "query-validators", QueryID: req.QueryID, CaseID: req.CaseID, Environment: req.Environment, InputNames: sortedKeys(req.Inputs), StartedAt: start, Actor: req.Actor, Steps: []StepResult{step}, ExecutionStatus: step.ExecutionStatus, Outcome: step.Outcome, Severity: step.Severity, DurationMs: time.Since(start).Milliseconds()}
	e.snapshot(&run, "", req.QueryID)
	run.CheckInvocations, run.QueryExecutions = e.nodes, e.queries
	return run, nil
}

// QueryParameters is the result of explicit scalar/set bindings. A range must
// be mapped to start and end parameters before reaching the query runner.
type QueryParameters = map[string]apicontract.TypedValueOrSet
