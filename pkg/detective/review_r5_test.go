package detective

import (
	"context"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/investigation"
)

func TestR5UnavailableValidatorFailSurvivesEveryEntryPath(t *testing.T) {
	ctx := context.Background()
	reader := mapReader{
		checks: map[string]Check{}, validators: map[string]PublishedValidator{},
		locals: map[string]QueryValidators{
			"q":     {QueryID: "q", Validators: []Validator{{ID: "required", Kind: "non-null", Scope: "row", Severity: "error", Field: "missing", OnUnavailable: "fail"}}},
			"after": {QueryID: "after", Validators: []Validator{{ID: "present", Kind: "non-null", Scope: "row", Severity: "info", Field: "x"}}},
			"list":  {QueryID: "list", Validators: []Validator{{ID: "each", Kind: "check", Scope: "row", Severity: "error", CheckID: "child"}}},
		},
		queries: map[string]bool{"q": true, "after": true, "list": true},
	}
	reader.checks["child"] = Check{ID: "child", Title: "Child", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q"}}}
	query := func(calls map[string]int) QueryRunner {
		return func(_ context.Context, id, _ string, _ map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
			calls[id]++
			return coreResult([]string{"x"}, [][]Value{{investigation.NewStringValue("one")}}), nil
		}
	}
	directCatalog, err := LoadCatalog(ctx, "", "q", reader)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := EvaluateQueryValidators(ctx, RunRequest{ID: "direct", QueryID: "q", Environment: "qa", Actor: "alice"}, directCatalog, query(map[string]int{}), nil)
	if err != nil || direct.Outcome != "fail" || direct.Steps[0].EvidenceOutcome != "unknown" || direct.Steps[0].Validators[0].Disposition != "fail" {
		t.Fatalf("direct policy result: %+v %v", direct, err)
	}
	for _, policy := range []string{"", "block", "fail", "warn", "skip"} {
		t.Run("step-"+policy, func(t *testing.T) {
			reader.checks["root"] = Check{ID: "root", Title: "Root", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q", OnUnavailable: policy}, {ID: "after", Kind: "query", QueryID: "after"}}}
			catalog, err := LoadCatalog(ctx, "root", "", reader)
			if err != nil {
				t.Fatal(err)
			}
			calls := map[string]int{}
			run, err := EvaluateCheck(ctx, RunRequest{ID: "wrapped", CheckID: "root", Environment: "qa", Actor: "alice"}, catalog, query(calls), nil)
			wantPolicy := effectivePolicy(policy)
			if err != nil || run.Outcome != "fail" || run.Steps[0].Outcome != "fail" || run.Steps[0].EvidenceOutcome != "unknown" || run.Steps[0].Disposition != wantPolicy || run.Steps[0].Validators[0].Outcome != "fail" || run.Steps[0].Validators[0].Disposition != "fail" {
				t.Fatalf("wrapped policy=%q run=%+v err=%v", policy, run, err)
			}
			if wantPolicy == "block" {
				if calls["after"] != 0 || run.Steps[1].ExecutionStatus != "skipped" {
					t.Fatalf("block did not stop later query: calls=%v steps=%+v", calls, run.Steps)
				}
			} else if calls["after"] != 1 || run.Steps[1].Outcome != "pass" {
				t.Fatalf("non-block stopped later query: calls=%v steps=%+v", calls, run.Steps)
			}
		})
	}
	reader.checks["nested"] = Check{ID: "nested", Title: "Nested", Steps: []Step{{ID: "child", Kind: "check", CheckID: "child"}}}
	nestedCatalog, err := LoadCatalog(ctx, "nested", "", reader)
	if err != nil {
		t.Fatal(err)
	}
	nested, err := EvaluateCheck(ctx, RunRequest{ID: "nested-run", CheckID: "nested", Environment: "qa", Actor: "alice"}, nestedCatalog, query(map[string]int{}), nil)
	if err != nil || nested.Outcome != "fail" || nested.Steps[0].Outcome != "fail" || nested.Steps[0].Children[0].Outcome != "fail" || nested.Steps[0].Children[0].Validators[0].Disposition != "fail" {
		t.Fatalf("nested policy result: %+v %v", nested, err)
	}
	listCatalog, err := LoadCatalog(ctx, "", "list", reader)
	if err != nil {
		t.Fatal(err)
	}
	perRow, err := EvaluateQueryValidators(ctx, RunRequest{ID: "per-row", QueryID: "list", Environment: "qa", Actor: "alice"}, listCatalog, query(map[string]int{}), nil)
	if err != nil || perRow.Outcome != "fail" || perRow.Steps[0].Validators[0].Outcome != "fail" || perRow.Steps[0].Validators[0].EvidenceOutcome != "unknown" || perRow.Steps[0].Validators[0].Children[0].Outcome != "fail" {
		t.Fatalf("per-row policy result: %+v %v", perRow, err)
	}
	reader.locals["q"] = QueryValidators{QueryID: "q", Validators: []Validator{
		{ID: "observed", Kind: "non-null", Scope: "row", Severity: "critical", Field: "x"},
		{ID: "unavailable", Kind: "non-null", Scope: "row", Severity: "error", Field: "missing", OnUnavailable: "fail"},
	}}
	reader.checks["root"] = Check{ID: "root", Title: "Root", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q"}}}
	observedCatalog, err := LoadCatalog(ctx, "root", "", reader)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := EvaluateCheck(ctx, RunRequest{ID: "observed-run", CheckID: "root", Environment: "qa", Actor: "alice"}, observedCatalog, func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		return coreResult([]string{"x"}, [][]Value{{investigation.NewNullValue()}}), nil
	}, nil)
	if err != nil || observed.Outcome != "fail" || observed.Steps[0].Outcome != "fail" || observed.Steps[0].EvidenceOutcome != "fail" || observed.Steps[0].Disposition != "block" {
		t.Fatalf("observed failure was erased by outer policy: %+v %v", observed, err)
	}
}
