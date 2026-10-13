package detective

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/investigation"
)

func TestSetValidatorsDistinguishObservedFailureFromIncompletePopulation(t *testing.T) {
	ctx := context.Background()
	e := &engine{}
	rows := coreResult([]string{"id", "total", "expected"}, [][]Value{
		{investigation.NewIntegerValue("1"), investigation.NewIntegerValue("5"), investigation.NewIntegerValue("5")},
		{investigation.NewIntegerValue("2"), investigation.NewIntegerValue("7"), investigation.NewIntegerValue("7")},
	}).Recordset
	for _, tc := range []struct {
		name      string
		validator Validator
		recordset apicontract.Recordset
		limited   bool
		want      string
	}{
		{"count-pass", Validator{ID: "count", Kind: "row-count", Expected: ptrValue(investigation.NewIntegerValue("2"))}, rows, false, "pass"},
		{"count-fail", Validator{ID: "count", Kind: "row-count", Expected: ptrValue(investigation.NewIntegerValue("3"))}, rows, false, "fail"},
		{"count-unknown-filtered", Validator{ID: "count", Kind: "row-count", Expected: ptrValue(investigation.NewIntegerValue("3"))}, rows, true, "unknown"},
		{"count-observed-excess", Validator{ID: "count", Kind: "row-count", Expected: ptrValue(investigation.NewIntegerValue("1"))}, rows, true, "fail"},
		{"count-missing-expectation", Validator{ID: "count", Kind: "row-count"}, rows, false, "unknown"},
		{"count-malformed-expectation", Validator{ID: "count", Kind: "row-count", Expected: ptrValue(investigation.NewIntegerValue("not-a-number"))}, rows, false, "unknown"},
		{"unique-pass", Validator{ID: "unique", Kind: "unique", Field: "id"}, rows, false, "pass"},
		{"unique-incomplete", Validator{ID: "unique", Kind: "unique", Field: "id"}, rows, true, "unknown"},
		{"unique-duplicate", Validator{ID: "unique", Kind: "unique", Field: "id"}, coreResult([]string{"id"}, [][]Value{{investigation.NewIntegerValue("1")}, {investigation.NewIntegerValue("1")}}).Recordset, false, "fail"},
		{"sum-pass", Validator{ID: "sum", Kind: "sum-equals", Field: "total", OtherField: "expected"}, rows, false, "pass"},
		{"sum-incomplete", Validator{ID: "sum", Kind: "sum-equals", Field: "total", OtherField: "expected"}, rows, true, "unknown"},
		{"sum-fail", Validator{ID: "sum", Kind: "sum-equals", Field: "total", OtherField: "expected"}, coreResult([]string{"total", "expected"}, [][]Value{{investigation.NewIntegerValue("5"), investigation.NewIntegerValue("7")}}).Recordset, false, "fail"},
		{"sum-not-numeric", Validator{ID: "sum", Kind: "sum-equals", Field: "total", OtherField: "expected"}, coreResult([]string{"total", "expected"}, [][]Value{{investigation.NewStringValue("private"), investigation.NewIntegerValue("7")}}).Recordset, false, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var limitations []apicontract.Limitation
			if tc.limited {
				limitations = []apicontract.Limitation{{RowsFiltered: true}}
			}
			got := e.validate(ctx, tc.validator, tc.recordset, false, limitations, nil, 0, nil)
			if len(got) != 1 || got[0].Outcome != tc.want {
				t.Fatalf("%s: %+v", tc.name, got)
			}
		})
	}
}

func TestRowValidatorsKeepPerRowIdentityAndFailClosedOnMissingEvidence(t *testing.T) {
	e := &engine{}
	rows := coreResult([]string{"id", "copy"}, [][]Value{{investigation.NewIntegerValue("1"), investigation.NewIntegerValue("1")}, {investigation.NewNullValue(), investigation.NewIntegerValue("2")}}).Recordset
	for _, tc := range []struct {
		validator Validator
		want      []string
	}{
		{Validator{ID: "required", Kind: "non-null", Field: "id"}, []string{"pass", "fail"}},
		{Validator{ID: "equals", Kind: "equals-fields", Field: "id", OtherField: "copy"}, []string{"pass", "fail"}},
	} {
		got := e.validate(context.Background(), tc.validator, rows, false, nil, nil, 0, nil)
		if len(got) != 2 || got[0].Outcome != tc.want[0] || got[1].Outcome != tc.want[1] || got[0].RowIndex == nil || *got[0].RowIndex != 0 || got[1].RowIndex == nil || *got[1].RowIndex != 1 {
			t.Fatalf("per-row identity/outcome: %+v", got)
		}
	}
	for _, tc := range []struct {
		field, other string
		limits       []apicontract.Limitation
	}{
		{"missing", "", nil},
		{"id", "", []apicontract.Limitation{{HiddenColumns: []string{"id"}}}},
		{"id", "missing", nil},
		{"id", "copy", []apicontract.Limitation{{HiddenColumns: []string{"copy"}}}},
	} {
		kind := "non-null"
		if tc.other != "" {
			kind = "equals-fields"
		}
		got := e.validate(context.Background(), Validator{ID: "required", Kind: kind, Field: tc.field, OtherField: tc.other, OnUnavailable: "fail"}, rows, false, tc.limits, nil, 0, nil)
		if len(got) != 1 || got[0].ExecutionStatus != "error" || got[0].EvidenceOutcome != "unknown" || got[0].Outcome != "fail" {
			t.Fatalf("missing or hidden field not marked unavailable: %+v", got)
		}
	}
	got := e.validate(context.Background(), Validator{ID: "required", Kind: "non-null", Field: "id"}, coreResult([]string{"id"}, nil).Recordset, false, nil, nil, 0, nil)
	if len(got) != 1 || got[0].Outcome != "unknown" {
		t.Fatalf("empty rows claimed health: %+v", got)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	got = e.validate(cancelled, Validator{ID: "required", Kind: "non-null", Field: "id"}, rows, false, nil, nil, 0, nil)
	if len(got) != 2 || got[0].ExecutionStatus != "skipped" || got[1].Disposition != "unattempted" {
		t.Fatalf("cancelled rows attempted: %+v", got)
	}
}

func TestUnavailablePoliciesPreserveObservedFailureAndSeverity(t *testing.T) {
	if EffectivePolicy("") != "block" || EffectivePolicy("warn") != "warn" {
		t.Fatal("unavailable policy default changed")
	}
	for _, policy := range []string{"block", "fail", "warn", "skip"} {
		step := StepResult{ExecutionStatus: "error", Outcome: "fail", EvidenceOutcome: "fail", Severity: "error"}
		applyUnavailable(&step, policy)
		if step.Outcome != "fail" || step.EvidenceOutcome != "fail" || step.Disposition != policy {
			t.Fatalf("policy erased observed failure: %+v", step)
		}
		validator := ValidationResult{ExecutionStatus: "error", Outcome: "unknown"}
		applyUnavailableValidation(&validator, policy)
		if validator.EvidenceOutcome != "unknown" || validator.Disposition != policy {
			t.Fatalf("policy hid uncertainty: %+v", validator)
		}
		if policy == "fail" && validator.Outcome != "fail" || policy != "fail" && validator.Outcome != "unknown" {
			t.Fatalf("policy outcome: %+v", validator)
		}
	}
	step := StepResult{Outcome: "unknown"}
	applyUnavailable(&step, "warn")
	if step.Severity != "warning" || step.EvidenceOutcome != "unknown" {
		t.Fatalf("warning lacks severity: %+v", step)
	}
	validator := ValidationResult{Outcome: "fail", Severity: "critical"}
	applyUnavailableValidation(&validator, "warn")
	if validator.Outcome != "fail" || validator.Severity != "critical" {
		t.Fatalf("warning reduced critical failure: %+v", validator)
	}
	if outcome, status, severity := AggregateSteps(nil); outcome != "unknown" || status != "completed" || severity != "" {
		t.Fatalf("empty aggregation: %s/%s/%s", outcome, status, severity)
	}
	if outcome, status, severity := AggregateSteps([]StepResult{{Outcome: "pass"}, {Outcome: "unknown", ExecutionStatus: "error", Severity: "warning"}, {Outcome: "fail", Severity: "critical"}}); outcome != "fail" || status != "error" || severity != "critical" {
		t.Fatalf("failure aggregation: %s/%s/%s", outcome, status, severity)
	}
	if aggregateStepEvidence(nil) != "unknown" || aggregateValidatorEvidence(nil) != "unknown" || aggregateStepEvidence([]StepResult{{Outcome: "fail"}}) != "fail" || aggregateValidatorEvidence([]ValidationResult{{Outcome: "fail"}}) != "fail" {
		t.Fatal("evidence aggregation lost unknown/fail")
	}
}

func TestEvaluatorRejectsInvalidRequestsAndKeepsSafeExecutionMetadata(t *testing.T) {
	ctx := context.Background()
	reader := mapReader{checks: map[string]Check{"root": {ID: "root", Title: "Root", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q", Validators: []Validator{{ID: "required", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}}}}}}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"q": true}}
	catalog, err := LoadCatalog(ctx, "root", "", reader)
	if err != nil {
		t.Fatal(err)
	}
	runner := func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		return coreResult([]string{"x"}, [][]Value{{investigation.NewStringValue("yes")}}), nil
	}
	for _, request := range []RunRequest{{ID: "Bad", CheckID: "root", Environment: "qa"}, {ID: "run", CheckID: "missing", Environment: "qa"}, {ID: "run", CheckID: "root", QueryID: "q", Environment: "qa"}, {ID: "run", CheckID: "root"}} {
		if _, err := EvaluateCheck(ctx, request, catalog, runner, nil); err == nil {
			t.Fatalf("invalid request accepted: %+v", request)
		}
	}
	if _, err := EvaluateCheck(ctx, RunRequest{ID: "run", CheckID: "root", Environment: "qa"}, catalog, nil, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil secure runner accepted: %v", err)
	}
	metadata := func(_ context.Context, ref *apicontract.ExecutionRef) (string, string, error) {
		if ref.ExecutionID != "receipt" {
			t.Fatalf("wrong execution ref: %+v", ref)
		}
		return "revision-1", "fingerprint-1", nil
	}
	withReceipt := func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		result, _ := runner(ctx, "q", "qa", nil)
		result.Execution = &apicontract.ExecutionRef{ProjectID: "p", StoreID: "evidence", ExecutionID: "receipt"}
		result.Provenance.Source = "saved-query"
		return result, nil
	}
	run, err := EvaluateCheck(ctx, RunRequest{ID: "run", CheckID: "root", Environment: "qa", Actor: "alice"}, catalog, withReceipt, metadata)
	if err != nil || run.Outcome != "pass" || run.Steps[0].QueryRevision != "revision-1" || run.Steps[0].ResultFingerprint != "fingerprint-1" || run.Steps[0].Source != "saved-query" {
		t.Fatalf("execution provenance lost: %+v %v", run, err)
	}
	for _, failure := range []struct {
		err      error
		wantCode string
	}{
		{&QueryFailure{Code: "SOURCE_UNAVAILABLE"}, "SOURCE_UNAVAILABLE"},
		{&QueryFailure{Code: "private://dsn"}, "INTERNAL"},
		{errors.New("private://dsn"), "INTERNAL"},
	} {
		run, err := EvaluateCheck(ctx, RunRequest{ID: "error-run", CheckID: "root", Environment: "qa"}, catalog, func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
			return apicontract.Result{}, failure.err
		}, nil)
		if err != nil || run.Steps[0].ErrorCode != failure.wantCode || strings.Contains(run.Steps[0].Message, "private://") {
			t.Fatalf("driver error leaked or code lost: %+v %v", run, err)
		}
	}
	if (&QueryFailure{Code: "SOURCE_UNAVAILABLE"}).Error() != "saved query execution failed" {
		t.Fatal("QueryFailure exposed driver detail")
	}
}
