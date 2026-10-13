package detective

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/investigation"
)

func TestR4MalformedLocalValidatorFailsBeforeExecution(t *testing.T) {
	reader := mapReader{checks: map[string]Check{}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{"q": {QueryID: "q", Validators: []Validator{{ID: "bad", Kind: "unsupported", Scope: "row", Severity: "error", Field: "x"}}}}, queries: map[string]bool{"q": true}}
	if _, err := LoadCatalog(context.Background(), "", "q", reader); !errors.Is(err, ErrInvalid) {
		t.Fatalf("malformed local catalog accepted: %v", err)
	}
	reader.locals["q"] = QueryValidators{QueryID: "q", Validators: []Validator{{ID: "known", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}}}
	catalog, err := LoadCatalog(context.Background(), "", "q", reader)
	if err != nil {
		t.Fatal(err)
	}
	delete(reader.locals, "q")
	got := catalog.QueryValidators["q"]
	if got.Validators[0].Kind != "non-null" {
		t.Fatal("catalog followed reader mutation")
	}
	unknown := (&engine{}).validate(context.Background(), Validator{ID: "bad", Kind: "unsupported", Scope: "row", Severity: "error", Field: "x"}, coreResult([]string{"x"}, [][]Value{{investigation.NewStringValue("ok")}}).Recordset, false, nil, nil, 0, nil)
	if len(unknown) != 1 || unknown[0].Outcome == "pass" || unknown[0].ExecutionStatus != "error" {
		t.Fatalf("unknown evaluator kind passed: %+v", unknown)
	}
	empty := (&engine{}).validate(context.Background(), Validator{ID: "bad", Kind: "unsupported", Scope: "row", Severity: "error", Field: "x"}, coreResult([]string{"x"}, nil).Recordset, false, nil, nil, 0, nil)
	if len(empty) != 1 || empty[0].ExecutionStatus != "error" || empty[0].Outcome == "pass" {
		t.Fatalf("unknown zero-row kind passed: %+v", empty)
	}
}

func TestR4DeepSnapshotsAndUnavailableDisposition(t *testing.T) {
	ctx := context.Background()
	reader := mapReader{checks: map[string]Check{}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{"q": {QueryID: "q", Validators: []Validator{{ID: "missing", Kind: "non-null", Scope: "row", Severity: "error", Field: "absent", OnUnavailable: "fail"}}}}, queries: map[string]bool{"q": true}}
	catalog, err := LoadCatalog(ctx, "", "q", reader)
	if err != nil {
		t.Fatal(err)
	}
	ref := &apicontract.ExecutionRef{ProjectID: "p", StoreID: "evidence", ExecutionID: "original"}
	result := coreResult([]string{"x"}, [][]Value{{investigation.NewStringValue("ok")}})
	result.Execution = ref
	run, err := EvaluateQueryValidatorsForResult(ctx, RunRequest{ID: "run-1", QueryID: "q", Environment: "qa", Actor: "alice"}, catalog, result, func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		t.Fatal("root query reexecuted")
		return apicontract.Result{}, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	v := run.Steps[0].Validators[0]
	if run.Outcome != "fail" || v.Outcome != "fail" || v.EvidenceOutcome != "unknown" || v.ExecutionStatus != "error" || v.Disposition != "fail" {
		t.Fatalf("missing field policy lost: run=%s validator=%+v", run.Outcome, v)
	}
	ref.ExecutionID = "rewritten"
	local := catalog.QueryValidators["q"]
	local.Validators[0].Field = "rewritten"
	if run.Steps[0].Execution.ExecutionID != "original" || run.QueryValidators[0].Validators[0].Field != "absent" {
		t.Fatalf("run evidence aliased caller or catalog: %+v", run)
	}
}

func TestR4CheckDefinitionSnapshotOwnsNestedFields(t *testing.T) {
	ctx := context.Background()
	expected := investigation.NewIntegerValue("1")
	root := Check{ID: "root", Title: "Root", Steps: []Step{{ID: "query", Kind: "query", QueryID: "q", Validators: []Validator{{ID: "count", Kind: "row-count", Scope: "set", Severity: "error", Expected: &expected}}}}}
	reader := mapReader{checks: map[string]Check{"root": root}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"q": true}}
	catalog, err := LoadCatalog(ctx, "root", "", reader)
	if err != nil {
		t.Fatal(err)
	}
	root.Steps[0].Validators[0].Expected.Str = "2"
	if catalog.Checks["root"].Steps[0].Validators[0].Expected.Str != "1" {
		t.Fatal("catalog retained authoring pointer")
	}
	run, err := EvaluateCheck(ctx, RunRequest{ID: "run", CheckID: "root", Environment: "qa", Actor: "alice"}, catalog, func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		return coreResult([]string{"x"}, [][]Value{{investigation.NewStringValue("ok")}}), nil
	}, nil)
	if err != nil || run.Outcome != "pass" {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	owned := catalog.Checks["root"]
	owned.Steps[0].Validators[0].Expected.Str = "3"
	if run.Definition.Steps[0].Validators[0].Expected.Str != "1" {
		t.Fatal("run definition retained catalog pointer")
	}
}

func TestR4ArchivedCaseEvidenceIntegrityAndScenarioReceipt(t *testing.T) {
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	ordinary := mapReader{checks: map[string]Check{}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"ordinary": true}}
	if _, err := LoadCatalog(context.Background(), "", "ordinary", ordinary); err != nil {
		t.Fatalf("ordinary saved query without Validators rejected: %v", err)
	}
	if _, _, err := CreateCase(Case{ID: "dangling", Title: "Case", Findings: []Finding{{ID: "finding", Text: "Confirmed", Status: "confirmed", EvidenceRefs: []string{"missing"}}}}, "alice", at); !errors.Is(err, ErrInvalid) {
		t.Fatalf("dangling evidence accepted: %v", err)
	}
	if _, _, err := CreateCase(Case{ID: "duplicate", Title: "Case", Evidence: []EvidenceRef{{ID: "evidence", Kind: "external", Ref: "one"}, {ID: "evidence", Kind: "external", Ref: "two"}}}, "alice", at); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate evidence accepted: %v", err)
	}
	preexisting, _, err := CreateCase(Case{ID: "collision", Title: "Case", Evidence: []EvidenceRef{{ID: "evidence-run", Kind: "external", Ref: "external"}}}, "alice", at)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := AttachRun(preexisting, CheckRun{ID: "run", CaseID: "collision"}, "event", "alice", at); !errors.Is(err, ErrInvalid) {
		t.Fatalf("server evidence ID collision accepted: %v", err)
	}
	c, _, err := CreateCase(Case{ID: "case", Title: "Case"}, "alice", at)
	if err != nil {
		t.Fatal(err)
	}
	scenario := Scenario{ID: "guide", Title: "Guide", Version: 1, Steps: []ScenarioStep{{ID: "inspect", Kind: "query", Ref: "ordinary"}}}
	applied, _, err := ApplyScenario(c, scenario, "application", "alice", at)
	if err != nil {
		t.Fatal(err)
	}
	scenario.Steps[0].Ref = "tampered"
	if applied.ScenarioApplications[0].Snapshot.Steps[0].Ref != "ordinary" {
		t.Fatal("scenario snapshot aliased authoring input")
	}
	proof := ScenarioQueryProof{Execution: apicontract.ExecutionRef{ProjectID: "p", StoreID: "evidence", ExecutionID: "execution"}, QueryID: "ordinary", Environment: "qa", ExecutionStatus: "running"}
	if _, _, err := RecordScenarioQueryStep(applied, "application", "inspect", proof, "alice", at); !errors.Is(err, ErrInvalid) {
		t.Fatalf("running ordinary query completed step: %v", err)
	}
	proof.ExecutionStatus = "completed"
	progress, _, err := RecordScenarioQueryStep(applied, "application", "inspect", proof, "alice", at)
	if err != nil || progress.ScenarioApplications[0].Steps[0].EvidenceOutcome != "unknown" || progress.ScenarioApplications[0].Steps[0].Execution == nil {
		t.Fatalf("ordinary receipt: %+v %v", progress, err)
	}
	proof.Execution.ExecutionID = "tampered"
	if progress.ScenarioApplications[0].Steps[0].Execution.ExecutionID != "execution" {
		t.Fatal("scenario receipt aliased proof")
	}
	closed := progress
	closed.Status, closed.ClosureReason = "closed", "resolved"
	closed, _, err = UpdateCase(progress, closed, "alice", at)
	if err != nil {
		t.Fatal(err)
	}
	archived := closed
	archived.Status = "archived"
	archived, _, err = UpdateCase(closed, archived, "alice", at)
	if err != nil {
		t.Fatal(err)
	}
	change := archived
	change.Title = "edited after archive"
	if _, _, err := UpdateCase(archived, change, "alice", at); !errors.Is(err, ErrInvalid) {
		t.Fatalf("archived edit accepted: %v", err)
	}
}

func TestR4ScenarioRunRequiresTerminalEvidence(t *testing.T) {
	at := time.Now().UTC()
	c, _, _ := CreateCase(Case{ID: "case", Title: "Case"}, "alice", at)
	scenario := Scenario{ID: "guide", Title: "Guide", Steps: []ScenarioStep{{ID: "inspect", Kind: "check", Ref: "check"}}}
	applied, _, err := ApplyScenario(c, scenario, "application", "alice", at)
	if err != nil {
		t.Fatal(err)
	}
	run := CheckRun{ID: "run", Kind: "check", CheckID: "check", CaseID: c.ID, ExecutionStatus: "running", Outcome: "unknown"}
	if _, _, err := RecordScenarioStep(applied, "application", "inspect", "completed", "run", &run, true, "alice", at); !errors.Is(err, ErrInvalid) {
		t.Fatalf("running run completed step: %v", err)
	}
	run.ExecutionStatus = "error"
	run.Outcome = "fail"
	if _, _, err := RecordScenarioStep(applied, "application", "inspect", "completed", "run", &run, true, "alice", at); !errors.Is(err, ErrInvalid) {
		t.Fatalf("execution error completed step: %v", err)
	}
	run.ExecutionStatus = "completed"
	progress, _, err := RecordScenarioStep(applied, "application", "inspect", "completed", "run", &run, true, "alice", at)
	if err != nil || progress.ScenarioApplications[0].Steps[0].EvidenceOutcome != "fail" {
		t.Fatalf("terminal failing assertion not reviewable: %+v %v", progress, err)
	}
}

func TestR4OperationReplayBindsScopeActorActionPayload(t *testing.T) {
	first, err := BindOperation("project", "alice", "save-check", "check", map[string]any{"a": 1, "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := BindOperation("project", "alice", "save-check", "check", map[string]any{"b": 2, "a": 1})
	if err != nil || first != reordered {
		t.Fatalf("canonical binding changed by map order: %+v %+v %v", first, reordered, err)
	}
	saved, err := NewOperationReceipt("operation", first, 2, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckOperationReplay(saved, reordered); err != nil {
		t.Fatalf("same replay rejected: %v", err)
	}
	for _, changed := range []OperationBinding{
		{Project: "other", Actor: first.Actor, Action: first.Action, ResourceID: first.ResourceID, PayloadDigest: first.PayloadDigest},
		{Project: first.Project, Actor: "bob", Action: first.Action, ResourceID: first.ResourceID, PayloadDigest: first.PayloadDigest},
		{Project: first.Project, Actor: first.Actor, Action: "delete-check", ResourceID: first.ResourceID, PayloadDigest: first.PayloadDigest},
		{Project: first.Project, Actor: first.Actor, Action: first.Action, ResourceID: "other", PayloadDigest: first.PayloadDigest},
	} {
		if !errors.Is(CheckOperationReplay(saved, changed), ErrConflict) {
			t.Fatalf("changed binding replayed: %+v", changed)
		}
	}
	changed, _ := BindOperation("project", "alice", "save-check", "check", map[string]any{"a": 2, "b": 2})
	if !errors.Is(CheckOperationReplay(saved, changed), ErrConflict) {
		t.Fatal("changed payload replayed")
	}
	saved.PayloadDigest = ""
	if !errors.Is(CheckOperationReplay(saved, first), ErrConflict) {
		t.Fatal("legacy unbound receipt replayed")
	}
}
