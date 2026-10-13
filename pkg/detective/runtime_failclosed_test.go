package detective

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/investigation"
)

func TestEngineStopsCancelledBoundedAndMissingWork(t *testing.T) {
	valid := Check{ID: "root", Title: "Root", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q"}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := engine{checks: map[string]Check{"root": valid}, cache: map[string]StepResult{}}
	if got := e.runCheck(ctx, "root", nil, 0, map[string]bool{}); got.ErrorCode != "TIMEOUT" || got.ExecutionStatus != "error" {
		t.Fatalf("cancelled Check continued: %+v", got)
	}
	for _, tc := range []struct {
		id       string
		depth    int
		nodes    int
		visiting map[string]bool
		message  string
	}{
		{"root", maxDepth + 1, 0, nil, "bound"},
		{"root", 0, maxNodes, nil, "bound"},
		{"root", 0, 0, map[string]bool{"root": true}, "cycle"},
		{"missing", 0, 0, nil, "unavailable"},
	} {
		e.nodes = tc.nodes
		got := e.runCheck(context.Background(), tc.id, nil, tc.depth, tc.visiting)
		if got.ExecutionStatus != "error" || got.Outcome != "unknown" {
			t.Fatalf("%s did not fail closed: %+v", tc.message, got)
		}
	}
	valid.Inputs = []Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}
	e.checks["root"] = valid
	if got := e.runCheck(context.Background(), "root", nil, 0, map[string]bool{}); got.ExecutionStatus != "error" {
		t.Fatalf("required input omitted: %+v", got)
	}
	invalidCache := make([]StepResult, 1)
	invalidCache[0].Children = invalidCache
	e.cache[cacheKey("root", "", map[string]InputValue{"id": {Scalar: ptrValue(investigation.NewIntegerValue("1"))}})] = StepResult{Children: invalidCache}
	if got := e.runCheck(context.Background(), "root", map[string]InputValue{"id": {Scalar: ptrValue(investigation.NewIntegerValue("1"))}}, 0, map[string]bool{}); got.ErrorCode != "INTERNAL" {
		t.Fatalf("corrupt cached evidence returned: %+v", got)
	}
}

func TestQueryStepKeepsMalformedAndMissingExecutionEvidenceUnknown(t *testing.T) {
	ctx := context.Background()
	step := Step{ID: "q", Kind: "query", QueryID: "q", Validators: []Validator{{ID: "required", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}}}
	good := func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		return coreResult([]string{"x"}, [][]Value{{investigation.NewStringValue("ok")}}), nil
	}
	e := engine{runner: good, cache: map[string]StepResult{}}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if got := e.runQueryStep(cancelled, step, nil, 0, nil); got.ErrorCode != "TIMEOUT" {
		t.Fatalf("cancelled query executed: %+v", got)
	}
	step.Bindings = []Binding{{Input: "id", FromInput: "missing"}}
	if got := e.runQueryStep(ctx, step, nil, 0, nil); got.ExecutionStatus != "error" || e.queries != 0 {
		t.Fatalf("missing query input executed: %+v", got)
	}
	step.Bindings = []Binding{{Input: "window", FromInput: "window"}}
	if got := e.runQueryStep(ctx, step, map[string]InputValue{"window": {Range: &ValueRange{Type: "range", Start: investigation.NewIntegerValue("1"), End: investigation.NewIntegerValue("2")}}}, 0, nil); got.ExecutionStatus != "error" || e.queries != 0 {
		t.Fatalf("range passed directly to saved query: %+v", got)
	}
	step.Bindings = nil
	for _, tc := range []struct {
		name   string
		runner QueryRunner
		code   string
	}{
		{"unserializable result", func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
			return coreResult([]string{"x"}, [][]Value{{{Type: investigation.ValueTypeNumber, Num: math.NaN()}}}), nil
		}, "INVALID_RECORDSET"},
		{"invalid row width", func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
			return coreResult([]string{"x"}, [][]Value{{investigation.NewStringValue("one"), investigation.NewStringValue("two")}}), nil
		}, "INVALID_RECORDSET"},
	} {
		e.runner = tc.runner
		got := e.runQueryStep(ctx, step, nil, 0, nil)
		if got.ErrorCode != tc.code || got.Outcome == "pass" {
			t.Fatalf("%s passed malformed evidence: %+v", tc.name, got)
		}
	}
	e.runner = good
	e.metadata = func(context.Context, *apicontract.ExecutionRef) (string, string, error) {
		return "", "", errors.New("metadata store unavailable")
	}
	if got := e.runQueryStep(ctx, step, nil, 0, nil); got.Outcome != "unknown" || got.Message != "execution receipt metadata is unavailable" {
		t.Fatalf("missing execution receipt passed: %+v", got)
	}
	e.runner = func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		result, _ := good(ctx, "q", "qa", nil)
		result.Execution = &apicontract.ExecutionRef{ProjectID: "p", StoreID: "evidence", ExecutionID: "receipt"}
		return result, nil
	}
	if got := e.runQueryStep(ctx, step, nil, 0, nil); got.Outcome != "unknown" || got.ExecutionStatus != "error" {
		t.Fatalf("failed receipt metadata passed: %+v", got)
	}
	step.Validators = []Validator{{ID: "ref", ValidatorID: "missing"}}
	e.metadata = nil
	if got := e.runQueryStep(ctx, step, nil, 0, nil); len(got.Validators) != 1 || got.Validators[0].ExecutionStatus != "error" {
		t.Fatalf("missing promoted Validator passed: %+v", got)
	}
	step.Validators = []Validator{{ID: "first", Kind: "non-null", Scope: "row", Severity: "error", Field: "missing"}, {ID: "later", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}}
	if got := e.runQueryStep(ctx, step, nil, 0, nil); len(got.Validators) != 2 || got.Validators[1].ExecutionStatus != "skipped" {
		t.Fatalf("unavailable Validator did not block later work: %+v", got)
	}
	step.Validators = []Validator{{ID: "required", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}}
	e.runner = func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		result, _ := good(ctx, "q", "qa", nil)
		result.Limitations = []apicontract.Limitation{{RowsFiltered: true}}
		return result, nil
	}
	if got := e.runQueryStep(ctx, step, nil, 0, nil); got.Outcome != "unknown" || got.EvidenceOutcome != "unknown" {
		t.Fatalf("filtered satisfying rows passed: %+v", got)
	}
}

func TestPerRowFanoutAndUnavailableBindingsStopSafely(t *testing.T) {
	rows := make([][]Value, maxFanout+1)
	for i := range rows {
		rows[i] = []Value{investigation.NewIntegerValue("1")}
	}
	e := engine{checks: map[string]Check{"child": {ID: "child", Title: "Child", Inputs: []Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}, Steps: []Step{{ID: "q", Kind: "query", QueryID: "q"}}}}, cache: map[string]StepResult{}, runner: func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		return apicontract.Result{}, &QueryFailure{Code: "SOURCE_UNAVAILABLE"}
	}}
	v := Validator{ID: "each", Kind: "check", Scope: "row", Severity: "error", CheckID: "child", Bindings: []Binding{{Input: "id", FromColumn: "id"}}}
	got := e.validate(context.Background(), v, coreResult([]string{"id"}, rows).Recordset, false, nil, nil, 0, map[string]bool{})
	if len(got) != 1 || got[0].ExecutionStatus != "skipped" || got[0].Outcome != "unknown" {
		t.Fatalf("fanout overflow attempted: %+v", got)
	}
	got = e.validate(context.Background(), v, coreResult([]string{"id"}, rows[:2]).Recordset, false, nil, nil, 0, map[string]bool{})
	if len(got) != 2 || got[0].Outcome != "unknown" || got[1].ExecutionStatus != "skipped" {
		t.Fatalf("missing child runner did not block row work: %+v", got)
	}
	v.Bindings[0].FromColumn = "missing"
	got = e.validate(context.Background(), v, coreResult([]string{"id"}, rows[:2]).Recordset, false, nil, nil, 0, map[string]bool{})
	if len(got) != 2 || got[0].ExecutionStatus != "error" || got[1].Disposition != "unattempted" {
		t.Fatalf("missing row binding did not stop safely: %+v", got)
	}
}

func TestSnapshotDeduplicatesSharedDefinitionsAndOrdersPromotions(t *testing.T) {
	e := engine{
		checks: map[string]Check{
			"root": {
				ID: "root", Title: "Root", Steps: []Step{
					{ID: "one", Kind: "check", CheckID: "shared"},
					{ID: "two", Kind: "check", CheckID: "shared"},
					{ID: "q", Kind: "query", QueryID: "query", Validators: []Validator{{ID: "z-ref", ValidatorID: "zeta"}, {ID: "a-ref", ValidatorID: "alpha"}}},
				},
			},
			"shared": {ID: "shared", Title: "Shared", Steps: []Step{{ID: "q", Kind: "query", QueryID: "query"}}},
		},
		validators: map[string]PublishedValidator{
			"zeta":  {ID: "zeta", Title: "Zeta", Validator: Validator{ID: "zeta", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}},
			"alpha": {ID: "alpha", Title: "Alpha", Validator: Validator{ID: "alpha", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}},
		},
		queryValidators: map[string]QueryValidators{"query": {QueryID: "query", Validators: []Validator{{ID: "z-ref", ValidatorID: "zeta"}}}},
	}
	var run CheckRun
	e.snapshot(&run, "root", "query")
	if len(run.Dependencies) != 1 || len(run.QueryValidators) != 1 || len(run.PublishedValidators) != 2 || run.PublishedValidators[0].ID != "alpha" {
		t.Fatalf("snapshot repeated or lost historical definition: %+v", run)
	}
}

func TestDirectEngineRejectsMissingNestedBindingBeforeChildQuery(t *testing.T) {
	e := engine{
		checks: map[string]Check{
			"root":  {ID: "root", Title: "Root", Steps: []Step{{ID: "child", Kind: "check", CheckID: "child", Bindings: []Binding{{Input: "id", FromInput: "missing"}}}}},
			"child": {ID: "child", Title: "Child", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q"}}},
		},
		cache: map[string]StepResult{},
		runner: func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
			t.Fatal("child query executed despite missing binding")
			return apicontract.Result{}, nil
		},
	}
	got := e.runCheck(context.Background(), "root", nil, 0, map[string]bool{})
	if len(got.Children) != 1 || got.Children[0].ExecutionStatus != "error" || e.queries != 0 {
		t.Fatalf("unavailable nested input executed: %+v", got)
	}
}
