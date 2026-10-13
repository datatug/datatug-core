package detective

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/investigation"
)

type mapReader struct {
	checks     map[string]Check
	validators map[string]PublishedValidator
	locals     map[string]QueryValidators
	queries    map[string]bool
}

func (m mapReader) GetCheck(_ context.Context, id string) (Check, error) {
	if v, ok := m.checks[id]; ok {
		return v, nil
	}
	return Check{}, ErrNotFound
}
func (m mapReader) GetPublishedValidator(_ context.Context, id string) (PublishedValidator, error) {
	if v, ok := m.validators[id]; ok {
		return v, nil
	}
	return PublishedValidator{}, ErrNotFound
}
func (m mapReader) GetQueryValidators(_ context.Context, id string) (QueryValidators, bool, error) {
	v, ok := m.locals[id]
	return v, ok, nil
}
func (m mapReader) QueryExists(_ context.Context, id string) (bool, error) { return m.queries[id], nil }

func coreResult(names []string, rows [][]Value) apicontract.Result {
	cols := make([]apicontract.Column, len(names))
	for i, name := range names {
		cols[i] = apicontract.Column{Name: name, Type: "string"}
	}
	return apicontract.Result{Recordset: apicontract.Recordset{Columns: cols, Rows: rows}}
}

func TestLoadCatalogAndBothEntryPathsEvaluateSameGraph(t *testing.T) {
	ctx := context.Background()
	m := mapReader{checks: map[string]Check{}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"list": true, "detail": true, "grand": true}}
	m.validators["published"] = PublishedValidator{ID: "published", Title: "Published", Version: 2, Validator: Validator{ID: "nonnull", Kind: "non-null", Scope: "row", Severity: "critical", Field: "x"}}
	m.checks["grand"] = Check{ID: "grand", Title: "Grand", Version: 3, Steps: []Step{{ID: "grand-query", Kind: "query", QueryID: "grand", Validators: []Validator{{ID: "reference", ValidatorID: "published"}}}}}
	m.checks["child"] = Check{ID: "child", Title: "Child", Version: 4, Steps: []Step{{ID: "detail", Kind: "query", QueryID: "detail"}, {ID: "grand", Kind: "check", CheckID: "grand"}}}
	m.locals["detail"] = QueryValidators{QueryID: "detail", Version: 5, Validators: []Validator{{ID: "local", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}}}
	m.locals["list"] = QueryValidators{QueryID: "list", Version: 6, Validators: []Validator{{ID: "each", Kind: "check", Scope: "row", Severity: "error", CheckID: "child"}}}
	runner := func(_ context.Context, id, _ string, _ map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		if id == "detail" {
			return coreResult([]string{"x"}, [][]Value{{investigation.NewNullValue()}}), nil
		}
		return coreResult([]string{"x"}, [][]Value{{investigation.NewIntegerValue("1")}}), nil
	}
	childCatalog, err := LoadCatalog(ctx, "child", "", m)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := EvaluateCheck(ctx, RunRequest{ID: "run-direct", CheckID: "child", Environment: "qa", Actor: "alice"}, childCatalog, runner, nil)
	if err != nil {
		t.Fatal(err)
	}
	queryCatalog, err := LoadCatalog(ctx, "", "list", m)
	if err != nil {
		t.Fatal(err)
	}
	via, err := EvaluateQueryValidators(ctx, RunRequest{ID: "run-via", QueryID: "list", Environment: "qa", Actor: "alice"}, queryCatalog, runner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if direct.Outcome != "fail" || via.Outcome != "fail" || len(via.Dependencies) != 2 || len(via.QueryValidators) != 2 || len(via.PublishedValidators) != 1 || via.PublishedValidators[0].Version != 2 {
		t.Fatalf("entry path or snapshot mismatch: direct=%+v via=%+v", direct, via)
	}
	if len(direct.QueryValidators) != 1 || len(direct.PublishedValidators) != 1 {
		t.Fatalf("direct snapshot incomplete: %+v", direct)
	}
	childCatalog.Checks["child"] = Check{ID: "child", Title: "Tampered", Steps: []Step{{ID: "missing", Kind: "query", QueryID: "missing"}}}
	if _, err := EvaluateCheck(ctx, RunRequest{ID: "run-tampered", CheckID: "child", Environment: "qa", Actor: "alice"}, childCatalog, runner, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("modified sealed catalog ran: %v", err)
	}
	// A later catalog edit cannot change an immutable run snapshot.
	m.validators["published"] = PublishedValidator{ID: "published", Title: "Changed", Version: 3}
	if via.PublishedValidators[0].Version != 2 {
		t.Fatal("snapshot aliased mutable catalog")
	}
}

func TestLoadCatalogRejectsMissingAndCyclicReferencesBeforeSource(t *testing.T) {
	ctx := context.Background()
	m := mapReader{checks: map[string]Check{}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"q": true}}
	m.checks["root"] = Check{ID: "root", Title: "Root", Steps: []Step{{ID: "child", Kind: "check", CheckID: "missing"}}}
	if _, err := LoadCatalog(ctx, "root", "", m); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing child=%v", err)
	}
	m.checks["root"] = Check{ID: "root", Title: "Root", Steps: []Step{{ID: "q", Kind: "query", QueryID: "missing"}}}
	if _, err := LoadCatalog(ctx, "root", "", m); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing query=%v", err)
	}
	m.checks["root"] = Check{ID: "root", Title: "Root", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q"}}}
	m.locals["q"] = QueryValidators{QueryID: "q", Validators: []Validator{{ID: "ref", ValidatorID: "missing"}}}
	if _, err := LoadCatalog(ctx, "root", "", m); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing promoted=%v", err)
	}
	m.locals["q"] = QueryValidators{QueryID: "q", Validators: []Validator{{ID: "child", Kind: "check", Scope: "row", Severity: "error", CheckID: "root"}}}
	if _, err := LoadCatalog(ctx, "root", "", m); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cycle via local=%v", err)
	}
	m.checks["root"] = Check{ID: "root", Title: "Root", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q"}}}
	m.locals["q"] = QueryValidators{QueryID: "q", Validators: []Validator{{ID: "child", Kind: "check", Scope: "row", Severity: "error", CheckID: "required"}}}
	m.checks["required"] = Check{ID: "required", Title: "Required", Inputs: []Input{{Name: "invoice_id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}, Steps: []Step{{ID: "q", Kind: "query", QueryID: "q"}}}
	if _, err := LoadCatalog(ctx, "root", "", m); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing child binding=%v", err)
	}
}

func TestCoreIncompleteEvidenceAndUnavailablePolicy(t *testing.T) {
	ctx := context.Background()
	count := investigation.NewIntegerValue("1")
	check := Check{ID: "root", Title: "Root", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q", Validators: []Validator{{ID: "count", Kind: "row-count", Scope: "set", Severity: "error", Expected: &count}}}}}
	reader := mapReader{checks: map[string]Check{"root": check}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"q": true, "other": true}}
	catalog, err := LoadCatalog(ctx, "root", "", reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"filtered", "truncated", "hidden"} {
		run, err := EvaluateCheck(ctx, RunRequest{ID: "run-" + mode, CheckID: "root", Environment: "qa", Actor: "alice"}, catalog, func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
			r := coreResult([]string{"x"}, [][]Value{{investigation.NewIntegerValue("1")}})
			if mode == "filtered" {
				r.Limitations = []apicontract.Limitation{{RowsFiltered: true}}
			}
			if mode == "truncated" {
				r.Truncated = true
			}
			if mode == "hidden" {
				r.Limitations = []apicontract.Limitation{{HiddenColumns: []string{"x"}}}
			}
			return r, nil
		}, nil)
		if err != nil || run.Outcome != "unknown" || run.Steps[0].Outcome == "pass" {
			t.Fatalf("%s false pass: %+v %v", mode, run, err)
		}
	}
	check.Steps = append(check.Steps, Step{ID: "later", Kind: "query", QueryID: "other"})
	reader.checks["root"] = check
	catalog, err = LoadCatalog(ctx, "root", "", reader)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	run, err := EvaluateCheck(ctx, RunRequest{ID: "run-unavailable", CheckID: "root", Environment: "qa", Actor: "alice"}, catalog, func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		calls++
		return apicontract.Result{}, &QueryFailure{Code: "SOURCE_UNAVAILABLE"}
	}, nil)
	if err != nil || calls != 1 || run.Outcome != "unknown" || len(run.Steps) != 2 || run.Steps[0].ErrorCode != "SOURCE_UNAVAILABLE" || run.Steps[0].EvidenceOutcome != "unknown" || run.Steps[0].Disposition != "block" || run.Steps[1].ExecutionStatus != "skipped" {
		t.Fatalf("default block=%+v calls=%d err=%v", run, calls, err)
	}
	for _, reversed := range []bool{false, true} {
		children := []StepResult{{Outcome: "fail", Severity: "error"}, {Outcome: "unknown", Severity: "critical"}}
		if reversed {
			children[0], children[1] = children[1], children[0]
		}
		outcome, _, severity := aggregate(children)
		if outcome != "fail" || severity != "critical" {
			t.Fatalf("order changed severity: %s/%s", outcome, severity)
		}
	}
}

func TestCorePerRowBlockAndNoAssertionUnknown(t *testing.T) {
	ctx := context.Background()
	m := mapReader{checks: map[string]Check{}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"list": true, "detail": true}}
	m.checks["child"] = Check{ID: "child", Title: "Child", Steps: []Step{{ID: "detail", Kind: "query", QueryID: "detail"}}}
	m.locals["list"] = QueryValidators{QueryID: "list", Validators: []Validator{{ID: "each", Kind: "check", Scope: "row", Severity: "error", CheckID: "child"}}}
	catalog, err := LoadCatalog(ctx, "", "list", m)
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]int{}
	run, err := EvaluateQueryValidators(ctx, RunRequest{ID: "run-list", QueryID: "list", Environment: "qa", Actor: "alice"}, catalog, func(_ context.Context, id, _ string, _ map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		calls[id]++
		if id == "detail" {
			return apicontract.Result{}, &QueryFailure{Code: "SOURCE_UNAVAILABLE"}
		}
		return coreResult([]string{"x"}, [][]Value{{investigation.NewIntegerValue("1")}, {investigation.NewIntegerValue("2")}, {investigation.NewIntegerValue("3")}}), nil
	}, nil)
	if err != nil || calls["detail"] != 1 || run.Outcome != "unknown" || len(run.Steps[0].Validators) != 3 || run.Steps[0].Validators[0].ExecutionStatus != "error" || run.Steps[0].Validators[1].ExecutionStatus != "skipped" || run.Steps[0].Validators[2].Disposition != "unattempted" {
		t.Fatalf("per-row block: run=%+v calls=%v err=%v", run, calls, err)
	}
	m.checks["plain"] = Check{ID: "plain", Title: "Plain", Steps: []Step{{ID: "detail", Kind: "query", QueryID: "detail"}}}
	plainCatalog, err := LoadCatalog(ctx, "plain", "", m)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := EvaluateCheck(ctx, RunRequest{ID: "run-plain", CheckID: "plain", Environment: "qa", Actor: "alice"}, plainCatalog, func(_ context.Context, _, _ string, _ map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		return coreResult([]string{"x"}, [][]Value{{investigation.NewIntegerValue("1")}}), nil
	}, nil)
	if err != nil || plain.Outcome != "unknown" {
		t.Fatalf("no-validator false pass: %+v %v", plain, err)
	}
}

func TestCoreValidatesExactExistingExecutionWithoutRootRerun(t *testing.T) {
	ctx := context.Background()
	m := mapReader{checks: map[string]Check{}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"list": true, "detail": true}}
	m.checks["child"] = Check{ID: "child", Title: "Child", Steps: []Step{{ID: "detail", Kind: "query", QueryID: "detail", Validators: []Validator{{ID: "nonnull", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}}}}}
	m.locals["list"] = QueryValidators{QueryID: "list", Validators: []Validator{{ID: "each", Kind: "check", Scope: "row", Severity: "error", CheckID: "child"}}}
	catalog, err := LoadCatalog(ctx, "", "list", m)
	if err != nil {
		t.Fatal(err)
	}
	execution := &apicontract.ExecutionRef{ProjectID: "p", StoreID: "evidence", ExecutionID: "exec-1"}
	retained := coreResult([]string{"x"}, [][]Value{{investigation.NewIntegerValue("1")}, {investigation.NewIntegerValue("2")}})
	retained.Execution = execution
	calls := map[string]int{}
	run, err := EvaluateQueryValidatorsForResult(ctx, RunRequest{ID: "run-existing", QueryID: "list", Environment: "qa", Actor: "alice"}, catalog, retained, func(_ context.Context, id, _ string, _ map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		calls[id]++
		return coreResult([]string{"x"}, [][]Value{{investigation.NewIntegerValue("1")}}), nil
	}, nil)
	if err != nil || calls["list"] != 0 || calls["detail"] != 1 || run.QueryExecutions != 1 || run.Steps[0].Execution == nil || *run.Steps[0].Execution != *execution {
		t.Fatalf("existing result correlation failed: run=%+v calls=%v err=%v", run, calls, err)
	}
	retained.Limitations = []apicontract.Limitation{{RowsFiltered: true}}
	limited, err := EvaluateQueryValidatorsForResult(ctx, RunRequest{ID: "run-limited", QueryID: "list", Environment: "qa", Actor: "alice"}, catalog, retained, func(_ context.Context, id, _ string, _ map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		if id == "list" {
			t.Fatal("root reexecuted")
		}
		return coreResult([]string{"x"}, [][]Value{{investigation.NewIntegerValue("1")}}), nil
	}, nil)
	if err != nil || limited.Outcome != "unknown" {
		t.Fatalf("limited retained rows passed: %+v %v", limited, err)
	}
	retained.Limitations = []apicontract.Limitation{{Policy: "snapshot-retention", HiddenColumns: []string{}}}
	limited, err = EvaluateQueryValidatorsForResult(ctx, RunRequest{ID: "run-retained", QueryID: "list", Environment: "qa", Actor: "alice"}, catalog, retained, func(_ context.Context, id, _ string, _ map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		if id == "list" {
			t.Fatal("root reexecuted")
		}
		return coreResult([]string{"x"}, [][]Value{{investigation.NewIntegerValue("1")}}), nil
	}, nil)
	if err != nil || limited.Outcome != "unknown" {
		t.Fatalf("restricted retained columns passed: %+v %v", limited, err)
	}
	retained.Execution = nil
	if _, err := EvaluateQueryValidatorsForResult(ctx, RunRequest{ID: "run-missing", QueryID: "list", Environment: "qa", Actor: "alice"}, catalog, retained, func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		t.Fatal("runner called without receipt")
		return apicontract.Result{}, nil
	}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing execution accepted: %v", err)
	}
}

func TestCoreCaseMutationsPreserveOwnershipAndAuditPrivacy(t *testing.T) {
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	c, a, err := CreateCase(Case{ID: "case-1", Title: "Investigate", ScenarioApplications: []ScenarioApplication{{ID: "forged"}}}, "alice", at)
	if err != nil || len(c.ScenarioApplications) != 0 || a.Action != "create-case" {
		t.Fatalf("create=%+v %+v %v", c, a, err)
	}
	scenario := Scenario{ID: "triage", Title: "Triage", Version: 2, Steps: []ScenarioStep{{ID: "query", Kind: "query", Ref: "q"}, {ID: "check", Kind: "check", Ref: "validate"}}}
	applied, a, err := ApplyScenario(c, scenario, "application-1", "alice", at.Add(time.Minute))
	if err != nil || applied.ScenarioApplications[0].ScenarioVersion != 2 || a.Action != "scenario-applied" {
		t.Fatalf("apply=%+v %+v %v", applied, a, err)
	}
	if len(c.ScenarioApplications) != 0 {
		t.Fatal("apply mutated source Case")
	}
	if _, _, err := RecordScenarioStep(applied, "application-1", "check", "skipped", "", nil, false, "alice", at); !errors.Is(err, ErrInvalid) {
		t.Fatalf("out-of-order step=%v", err)
	}
	run := CheckRun{ID: "run-1", Kind: "query-validators", QueryID: "q", CaseID: c.ID, ExecutionStatus: "completed", Outcome: "unknown", StartedAt: at, Actor: "alice"}
	linked, _, err := AttachRun(applied, run, "event-1", "alice", at.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := RecordScenarioStep(linked, "application-1", "query", "completed", "run-1", &run, false, "alice", at.Add(3*time.Minute)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unverified run link accepted: %v", err)
	}
	progress, a, err := RecordScenarioStep(linked, "application-1", "query", "completed", "run-1", &run, true, "alice", at.Add(3*time.Minute))
	if err != nil || progress.ScenarioApplications[0].Steps[0].RunID != "run-1" || a.Action != "scenario-step-completed" {
		t.Fatalf("receipt=%+v %+v %v", progress, a, err)
	}
	if linked.ScenarioApplications[0].Steps[0].Status != "pending" {
		t.Fatal("receipt mutated input Case")
	}
	if _, _, err := AppendCaseEvent(progress, CaseEvent{Type: "closed", Message: "forged"}, "event-2", nil, "alice", at); !errors.Is(err, ErrInvalid) {
		t.Fatalf("forged lifecycle=%v", err)
	}
	noted, a, err := AppendCaseEvent(progress, CaseEvent{ID: "client-forgery", At: at.Add(-time.Hour), Actor: "mallory", Type: "note", Message: "Investigating"}, "event-2", nil, "alice", at.Add(4*time.Minute))
	if err != nil || noted.Timeline[len(noted.Timeline)-1].Actor != "alice" || noted.Timeline[len(noted.Timeline)-1].ID != "event-2" || a.Action != "note" {
		t.Fatalf("note=%+v %+v %v", noted, a, err)
	}
	next := noted
	next.Status = "closed"
	next.ClosureReason = "confirmed mismatch"
	next.RunRefs = nil
	next.ScenarioApplications = nil
	closed, a, err := UpdateCase(noted, next, "alice", at.Add(5*time.Minute))
	if err != nil || closed.RunRefs[0] != "run-1" || len(closed.ScenarioApplications) != 1 || a.Action != "update-case" {
		t.Fatalf("close=%+v %+v %v", closed, a, err)
	}
	serialized, _ := json.Marshal(AuditForRun(run))
	if strings.Contains(string(serialized), "Investigating") || strings.Contains(string(serialized), "invoice_id") {
		t.Fatal("audit leaked context")
	}
}

func TestCoreOrderedTypedRanges(t *testing.T) {
	for _, r := range []ValueRange{{Type: "range", Start: investigation.NewDateValue("2026-10-14"), End: investigation.NewDateValue("2026-10-13")}, {Type: "range", Start: investigation.NewDatetimeValue("2026-10-13T00:00:00.1Z"), End: investigation.NewDatetimeValue("2026-10-13T00:00:00Z")}, {Type: "range", Start: investigation.NewIntegerValue("10"), End: investigation.NewIntegerValue("1")}, {Type: "range", Start: investigation.NewStringValue("a"), End: investigation.NewStringValue("z")}} {
		if (InputValue{Range: &r}).Validate() == nil {
			t.Fatalf("accepted invalid range: %+v", r)
		}
	}
}
