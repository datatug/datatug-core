package detective

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/investigation"
)

func TestInputCardinalityAndMappingRejectUnknownOrMismatchedValues(t *testing.T) {
	integer := investigation.NewIntegerValue("1")
	schema := []Input{{Name: "scalar", Type: investigation.ValueTypeInteger, Cardinality: "scalar"}, {Name: "set", Type: investigation.ValueTypeInteger, Cardinality: "set"}, {Name: "range", Type: investigation.ValueTypeDate, Cardinality: "range"}}
	set := InputValue{Set: &apicontract.TypedValueSet{Values: []Value{integer}}}
	rangeValue := InputValue{Range: &ValueRange{Type: "range", Start: investigation.NewDateValue("2026-10-01"), End: investigation.NewDateValue("2026-10-02")}}
	if err := ValidateInputs(schema, map[string]InputValue{"scalar": {Scalar: &integer}, "set": set, "range": rangeValue}); err != nil {
		t.Fatal(err)
	}
	for _, values := range []map[string]InputValue{
		{"unknown": {Scalar: &integer}},
		{"scalar": {}},
		{"set": {Scalar: &integer}},
		{"set": {Set: &apicontract.TypedValueSet{Values: []Value{investigation.NewStringValue("1")}}}},
		{"range": set},
		{"range": {Range: &ValueRange{Type: "range", Start: integer, End: integer}}},
	} {
		if err := ValidateInputs(schema, values); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid typed input accepted: %+v %v", values, err)
		}
	}
	if !ValidParameterName("A1_B") || ValidParameterName("a/") || ValidParameterName(strings.Repeat("a", 129)) || validColumnName(strings.Repeat("a", 129)) || validExecutionCode("mixedCase") || validExecutionCode("") {
		t.Fatal("identifier bounds changed")
	}
	if got, ok := numeric(investigation.NewNumberValue(1.5)); !ok || got.RatString() != "3/2" {
		t.Fatalf("floating number not converted exactly enough: %v %v", got, ok)
	}
	if !validColumnName("Invoice total") {
		t.Fatal("read-only display column unexpectedly rejected")
	}
}

func TestRangeJsonAndCloneRejectInvalidTypedData(t *testing.T) {
	for _, encoded := range []string{
		`{"type":"range","start":{"type":"date","value":"not-a-date"},"end":{"type":"date","value":"2026-10-02"}}`,
		`{"type":"range","start":{"type":"integer","value":"bad"},"end":{"type":"integer","value":"2"}}`,
		`{"type":"set","values":[]}`,
		`{"type":"integer","value":"bad"}`,
	} {
		var value InputValue
		if err := json.Unmarshal([]byte(encoded), &value); err == nil {
			t.Fatalf("invalid typed context decoded: %s", encoded)
		}
	}
	if _, err := cloneValue(apicontract.TypedValueSet{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid set copied into immutable evidence: %v", err)
	}
	if _, err := cloneValue(math.NaN()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unserializable number copied into evidence: %v", err)
	}
	if _, _, err := CreateCase(Case{ID: "case", Title: "Case", Context: map[string]InputValue{"bad": {}}}, "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unserializable Case context accepted: %v", err)
	}
}

func TestOperationBindingRejectsInvalidPayloadAndReceipt(t *testing.T) {
	if _, err := BindOperation("", "alice", "save-check", "check", map[string]string{"x": "y"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unscoped operation accepted: %v", err)
	}
	if _, err := BindOperation("project", "alice", "save-check", "check", math.NaN()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unserializable payload accepted: %v", err)
	}
	if _, err := BindOperation("project", "alice", "save-check", "check", json.RawMessage(`{"type":`)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid raw payload accepted: %v", err)
	}
	binding, err := BindOperation("project", "alice", "save-check", "check", map[string]any{"value": 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, receipt := range []OperationReceipt{
		{ID: "", Project: "project", Actor: "alice", Action: "save-check", PayloadDigest: binding.PayloadDigest, At: lifecycleAt},
		{ID: "id", Project: "project", Actor: "alice", Action: "save-check", ResourceVersion: -1, PayloadDigest: binding.PayloadDigest, At: lifecycleAt},
		{ID: "id", Project: "project", Actor: "alice", Action: "save-check", PayloadDigest: strings.Repeat("x", 64), At: lifecycleAt},
	} {
		if err := receipt.Validate(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid operation receipt accepted: %+v %v", receipt, err)
		}
	}
}

func TestOperationBindingRejectsPayloadThatOverflowsScopeEnvelope(t *testing.T) {
	var first any = "first"
	var second any = "second"
	for range 10000 {
		first = []any{first}
		second = []any{second}
	}
	if _, err := json.Marshal(first); err != nil {
		t.Fatalf("payload must be valid before the scope envelope is added: %v", err)
	}
	for _, payload := range []any{first, second} {
		binding, err := BindOperation("project", "alice", "update-case", "case", payload)
		if !errors.Is(err, ErrInvalid) || binding != (OperationBinding{}) {
			t.Fatalf("unhashable scoped payload produced a replay digest: %+v %v", binding, err)
		}
	}
}

func TestDefinitionCloneAndEvaluationRejectCorruptedCatalogs(t *testing.T) {
	ctx := context.Background()
	reader := mapReader{checks: map[string]Check{"root": {ID: "root", Title: "Root", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q"}}}}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"q": true}}
	catalog, err := LoadCatalog(ctx, "root", "", reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EvaluateCheck(ctx, RunRequest{ID: "run", CheckID: "root", Environment: "qa", Inputs: map[string]InputValue{"unknown": {Scalar: ptrValue(investigation.NewIntegerValue("1"))}}}, catalog, func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		return apicontract.Result{}, nil
	}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown root input reached saved query: %v", err)
	}
	delete(catalog.Checks, "root")
	catalog.signature = catalogSignature(catalog)
	if _, err := EvaluateCheck(ctx, RunRequest{ID: "run", CheckID: "root", Environment: "qa"}, catalog, func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		return apicontract.Result{}, nil
	}, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent selected Check accepted: %v", err)
	}
	badValue := Value{Type: investigation.ValueTypeNumber, Num: math.NaN()}
	reader.checks["root"] = Check{ID: "root", Title: "Root", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q", Validators: []Validator{{ID: "required", Kind: "non-null", Scope: "row", Severity: "error", Field: "x", Expected: &badValue}}}}}
	if _, err := LoadCatalog(ctx, "root", "", reader); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unserializable Check catalog accepted: %v", err)
	}
	reader.locals["q"] = QueryValidators{QueryID: "q", Validators: []Validator{{ID: "required", Kind: "non-null", Scope: "row", Severity: "error", Field: "x", Expected: &badValue}}}
	reader.checks["root"] = Check{ID: "root", Title: "Root", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q"}}}
	if _, err := LoadCatalog(ctx, "", "q", reader); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unserializable local catalog accepted: %v", err)
	}
	reader.locals["q"] = QueryValidators{QueryID: "q", Validators: []Validator{{ID: "ref", ValidatorID: "published"}}}
	reader.validators["published"] = PublishedValidator{ID: "published", Title: "Published", Validator: Validator{ID: "required", Kind: "non-null", Scope: "row", Severity: "error", Field: "x", Expected: &badValue}}
	if _, err := LoadCatalog(ctx, "", "q", reader); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unserializable promoted catalog accepted: %v", err)
	}
}

func TestQueryValidatorEvaluationRejectsUnboundOrUnownedExecution(t *testing.T) {
	ctx := context.Background()
	reader := mapReader{checks: map[string]Check{}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{"q": {QueryID: "q", Validators: []Validator{{ID: "required", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}}}}, queries: map[string]bool{"q": true}}
	catalog, err := LoadCatalog(ctx, "", "q", reader)
	if err != nil {
		t.Fatal(err)
	}
	runner := func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		return coreResult([]string{"x"}, [][]Value{{investigation.NewStringValue("ok")}}), nil
	}
	for _, request := range []RunRequest{{ID: "Bad", QueryID: "q", Environment: "qa"}, {ID: "run", QueryID: "../escape", Environment: "qa"}, {ID: "run", QueryID: "q", CheckID: "root", Environment: "qa"}, {ID: "run", QueryID: "q"}} {
		if _, err := EvaluateQueryValidators(ctx, request, catalog, runner, nil); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid query Validator request accepted: %+v %v", request, err)
		}
	}
	if _, err := EvaluateQueryValidators(ctx, RunRequest{ID: "run", QueryID: "q", Environment: "qa"}, catalog, nil, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil secure runner accepted: %v", err)
	}
	for _, value := range []InputValue{{Range: &ValueRange{Type: "range", Start: investigation.NewIntegerValue("1"), End: investigation.NewIntegerValue("2")}}, {}} {
		if _, err := EvaluateQueryValidators(ctx, RunRequest{ID: "run", QueryID: "q", Environment: "qa", Inputs: map[string]InputValue{"id": value}}, catalog, runner, nil); !errors.Is(err, ErrInvalid) {
			t.Fatalf("unbound query parameter accepted: %+v %v", value, err)
		}
	}
	run, err := EvaluateQueryValidators(ctx, RunRequest{ID: "run", QueryID: "q", Environment: "qa", Inputs: map[string]InputValue{"id": {Scalar: ptrValue(investigation.NewIntegerValue("1"))}}}, catalog, runner, nil)
	if err != nil || run.InputNames[0] != "id" {
		t.Fatalf("typed query input names lost: %+v %v", run, err)
	}
	result := coreResult([]string{"x"}, [][]Value{{investigation.NewStringValue("ok")}})
	result.Execution = &apicontract.ExecutionRef{ProjectID: "p", StoreID: "evidence", ExecutionID: "receipt"}
	if _, err := EvaluateQueryValidatorsForResult(ctx, RunRequest{ID: "run", QueryID: "q", Environment: "qa"}, catalog, apicontract.Result{}, runner, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unreceipted rows accepted: %v", err)
	}
	result.Recordset.Rows = [][]Value{{investigation.NewStringValue("one"), investigation.NewStringValue("two")}}
	if _, err := EvaluateQueryValidatorsForResult(ctx, RunRequest{ID: "run", QueryID: "q", Environment: "qa"}, catalog, result, runner, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("malformed retained rows accepted: %v", err)
	}
	reader.locals["q"] = QueryValidators{QueryID: "q", Validators: []Validator{{ID: "ref", ValidatorID: "published"}}}
	reader.validators["published"] = PublishedValidator{ID: "published", Title: "Published", Validator: Validator{ID: "required", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}}
	catalog, err = LoadCatalog(ctx, "", "q", reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EvaluateQueryValidators(ctx, RunRequest{ID: "run", QueryID: "q", Environment: "qa"}, catalog, runner, nil); err != nil {
		t.Fatalf("promoted query Validator not executable: %v", err)
	}
}

func TestEvaluationRejectsTamperedOrUnserializableCatalogAndResults(t *testing.T) {
	ctx := context.Background()
	check := Check{ID: "root", Title: "Root", Steps: []Step{{ID: "query", Kind: "query", QueryID: "q"}}}
	reader := mapReader{checks: map[string]Check{"root": check}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{"q": {QueryID: "q", Validators: []Validator{{ID: "required", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}}}}, queries: map[string]bool{"q": true}}
	checkCatalog, err := LoadCatalog(ctx, "root", "", reader)
	if err != nil {
		t.Fatal(err)
	}
	queryCatalog, err := LoadCatalog(ctx, "", "q", reader)
	if err != nil {
		t.Fatal(err)
	}
	runner := func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		return coreResult([]string{"x"}, [][]Value{{investigation.NewStringValue("yes")}}), nil
	}
	checkCatalog.Checks["root"] = Check{ID: "root", Title: "Root", Steps: []Step{{ID: "missing", Kind: "check", CheckID: "missing"}}}
	checkCatalog.signature = catalogSignature(checkCatalog)
	if _, err := EvaluateCheck(ctx, RunRequest{ID: "run", CheckID: "root", Environment: "qa"}, checkCatalog, runner, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tampered Check graph accepted: %v", err)
	}
	queryCatalog.QueryValidators["q"] = QueryValidators{QueryID: "q", Validators: []Validator{{ID: "missing", ValidatorID: "missing"}}}
	queryCatalog.signature = catalogSignature(queryCatalog)
	if _, err := EvaluateQueryValidators(ctx, RunRequest{ID: "run", QueryID: "q", Environment: "qa"}, queryCatalog, runner, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tampered query Validator graph accepted: %v", err)
	}
	badValue := Value{Type: investigation.ValueTypeNumber, Num: math.NaN()}
	check.Steps[0].Validators = []Validator{{ID: "required", Kind: "non-null", Scope: "row", Severity: "error", Field: "x", Expected: &badValue}}
	checkCatalog = Catalog{Checks: map[string]Check{"root": check}, Validators: map[string]PublishedValidator{}, QueryValidators: map[string]QueryValidators{}, verified: true}
	checkCatalog.signature = catalogSignature(checkCatalog)
	if _, err := EvaluateCheck(ctx, RunRequest{ID: "run", CheckID: "root", Environment: "qa"}, checkCatalog, runner, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unserializable Check snapshot accepted: %v", err)
	}
	queryCatalog = Catalog{Checks: map[string]Check{}, Validators: map[string]PublishedValidator{}, QueryValidators: map[string]QueryValidators{"q": {QueryID: "q", Validators: check.Steps[0].Validators}}, verified: true}
	queryCatalog.signature = catalogSignature(queryCatalog)
	if _, err := EvaluateQueryValidators(ctx, RunRequest{ID: "run", QueryID: "q", Environment: "qa"}, queryCatalog, runner, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unserializable query Validator snapshot accepted: %v", err)
	}
	goodCatalog, err := LoadCatalog(ctx, "", "q", reader)
	if err != nil {
		t.Fatal(err)
	}
	result := coreResult([]string{"x"}, [][]Value{{investigation.NewStringValue("yes")}})
	result.Execution = &apicontract.ExecutionRef{ProjectID: "p", StoreID: "evidence", ExecutionID: "receipt"}
	result.BindingsApplied = []apicontract.Binding{{ParameterID: "id", Value: apicontract.TypedValueOrSet{}}}
	if _, err := EvaluateQueryValidatorsForResult(ctx, RunRequest{ID: "run", QueryID: "q", Environment: "qa"}, goodCatalog, result, runner, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("malformed retained binding copied into evidence: %v", err)
	}
}

func TestQueryLocalCheckInputPreflightRequiresDeclaredSource(t *testing.T) {
	ctx := context.Background()
	reader := mapReader{
		checks:     map[string]Check{"child": {ID: "child", Title: "Child", Inputs: []Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}, Steps: []Step{{ID: "q2", Kind: "query", QueryID: "q2"}}}},
		validators: map[string]PublishedValidator{},
		locals:     map[string]QueryValidators{"q": {QueryID: "q", Validators: []Validator{{ID: "child", Kind: "check", Scope: "row", Severity: "error", CheckID: "child", Bindings: []Binding{{Input: "id", FromInput: "invoice_id"}}}}}},
		queries:    map[string]bool{"q": true, "q2": true},
	}
	catalog, err := LoadCatalog(ctx, "", "q", reader)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	if _, err := EvaluateQueryValidators(ctx, RunRequest{ID: "run", QueryID: "q", Environment: "qa"}, catalog, func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		called = true
		return apicontract.Result{}, nil
	}, nil); !errors.Is(err, ErrInvalid) || called {
		t.Fatalf("unbound parent input reached query runner: %v called=%t", err, called)
	}
}

func TestRetainedExecutionCanBeUsedOnlyForSelectedQueryAndEnvironment(t *testing.T) {
	ctx := context.Background()
	retained := coreResult([]string{"x"}, [][]Value{{investigation.NewStringValue("retained")}})
	calls := 0
	runner := withExistingResult(RunRequest{QueryID: "invoice", Environment: "qa"}, &retained, func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		calls++
		return coreResult([]string{"x"}, [][]Value{{investigation.NewStringValue("nested")}}), nil
	})
	if _, err := runner(ctx, "other", "qa", nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("retained rows accepted for another query: %v", err)
	}
	if _, err := runner(ctx, "invoice", "qa", nil); err != nil || calls != 1 {
		t.Fatalf("rejected receipt reused instead of secured runner: %v calls=%d", err, calls)
	}
	runner = withExistingResult(RunRequest{QueryID: "invoice", Environment: "qa"}, &retained, func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		t.Fatal("source executed despite retained receipt")
		return apicontract.Result{}, nil
	})
	if _, err := runner(ctx, "invoice", "prod", nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("retained rows accepted for another environment: %v", err)
	}
}

func TestQueryCatalogWithoutAssertionsIsNotRunnableValidation(t *testing.T) {
	reader := mapReader{checks: map[string]Check{}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"ordinary": true}}
	catalog, err := LoadCatalog(context.Background(), "", "ordinary", reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EvaluateQueryValidators(context.Background(), RunRequest{ID: "run", QueryID: "ordinary", Environment: "qa"}, catalog, func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		return coreResult([]string{"x"}, nil), nil
	}, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ordinary query fabricated Validator pass: %v", err)
	}
}
