package detective

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/investigation"
)

func TestInputValuesRoundTripAndRejectAmbiguousOrUnorderedValues(t *testing.T) {
	integer := investigation.NewIntegerValue("7")
	values := []InputValue{
		{Scalar: &integer},
		{Set: &apicontract.TypedValueSet{Values: []Value{integer}}},
		{Range: &ValueRange{Type: "range", Start: investigation.NewIntegerValue("1"), End: integer}},
	}
	for _, original := range values {
		encoded, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		var decoded InputValue
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Validate() != nil {
			t.Fatalf("valid value lost in round trip: %s", encoded)
		}
	}
	bad := []InputValue{
		{}, {Scalar: &integer, Range: &ValueRange{Type: "range"}},
		{Range: &ValueRange{Type: "range", Start: integer, End: investigation.NewIntegerValue("1")}},
		{Range: &ValueRange{Type: "range", Start: investigation.NewStringValue("a"), End: investigation.NewStringValue("z")}},
		{Range: &ValueRange{Type: "range", Start: investigation.NewIntegerValue("1"), End: investigation.NewStringValue("1")}},
		{Range: &ValueRange{Type: "not-range", Start: integer, End: integer}},
	}
	for _, value := range bad {
		if err := value.Validate(); err == nil {
			t.Fatalf("invalid context value accepted: %+v", value)
		}
		if _, err := json.Marshal(value); err == nil {
			t.Fatalf("invalid context value serialized: %+v", value)
		}
	}
	for _, encoded := range []string{`{`, `{"type":"set","values":[]}`, `{"type":"range","start":{"type":"integer","value":"9"},"end":{"type":"integer","value":"1"}}`, `{"type":"not-a-type"}`} {
		var value InputValue
		if err := json.Unmarshal([]byte(encoded), &value); err == nil {
			t.Fatalf("invalid wire input accepted: %s", encoded)
		}
	}
}

func TestPublicDefinitionValidationRejectsMalformedRunnableAssets(t *testing.T) {
	base := Check{ID: "check", Title: "Check", Inputs: []Input{{Name: "invoice_id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}, Steps: []Step{{ID: "q", Kind: "query", QueryID: "queries/invoice", Bindings: []Binding{{Input: "invoice_id", FromInput: "invoice_id"}}}}}
	if err := ValidateCheck(base); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*Check){
		func(c *Check) { c.ID = "Bad" }, func(c *Check) { c.Title = " " }, func(c *Check) { c.Steps = nil },
		func(c *Check) { c.Inputs[0].Name = "invoice id" }, func(c *Check) { c.Inputs[0].Type = "unsupported" },
		func(c *Check) { c.Inputs[0].Cardinality = "many" }, func(c *Check) { c.Inputs = append(c.Inputs, c.Inputs[0]) },
		func(c *Check) { c.Steps[0].ID = "Bad" }, func(c *Check) { c.Steps[0].Kind = "unknown" },
		func(c *Check) { c.Steps[0].QueryID = "" }, func(c *Check) { c.Steps[0].CheckID = "child" },
		func(c *Check) { c.Steps[0].OnUnavailable = "ignore" },
		func(c *Check) { c.Steps = append(c.Steps, c.Steps[0]) },
		func(c *Check) { c.Steps[0].Bindings = []Binding{{Input: "bad name", FromInput: "invoice_id"}} },
		func(c *Check) { c.Steps[0].Validators = []Validator{{ID: "bad", Kind: "unsupported"}} },
		func(c *Check) { c.Steps[0] = Step{ID: "child", Kind: "check", CheckID: "Bad"} },
		func(c *Check) {
			c.Steps[0] = Step{ID: "child", Kind: "check", CheckID: "child", Validators: []Validator{{ID: "v"}}}
		},
	}
	for i, mutate := range mutations {
		candidate := base
		candidate.Inputs = append([]Input(nil), base.Inputs...)
		candidate.Steps = append([]Step(nil), base.Steps...)
		mutate(&candidate)
		if err := ValidateCheck(candidate); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid Check variant %d accepted: %v", i, err)
		}
	}
	if err := ValidateCheck(Check{ID: "parent", Title: "Parent", Steps: []Step{{ID: "child", Kind: "check", CheckID: "child"}}}); err != nil {
		t.Fatalf("valid nested Check rejected: %v", err)
	}
	if !ValidSlug("invoice-check") || ValidSlug("Invoice_Check") || ValidSlug(strings.Repeat("a", 129)) || !ValidParameterName("CustomerId") || !ValidParameterName("invoice_id") || ValidParameterName("0id") || ValidParameterName("bad/name") || !ValidQueryRef("dir/invoice_detail.sql") || ValidQueryRef("../escape") || ValidQueryRef("foo//bar") || !ValidInputType(investigation.ValueTypeDate) || ValidInputType("opaque") {
		t.Fatal("resource and input namespaces were confused")
	}
}

func TestValidatorDefinitionScopeEvidenceAndPolicy(t *testing.T) {
	wanted := investigation.NewIntegerValue("2")
	valid := []Validator{
		{ID: "nn", Kind: "non-null", Scope: "row", Severity: "error", Field: "invoice_id"},
		{ID: "eq", Kind: "equals-fields", Scope: "row", Severity: "warning", Field: "total", OtherField: "expected"},
		{ID: "count", Kind: "row-count", Scope: "set", Severity: "critical", Expected: &wanted},
		{ID: "unique", Kind: "unique", Scope: "set", Severity: "error", Field: "invoice_id"},
		{ID: "sum", Kind: "sum-equals", Scope: "set", Severity: "error", Field: "total", OtherField: "expected"},
		{ID: "child", Kind: "check", Scope: "row", Severity: "error", CheckID: "child"},
		{ID: "ref", ValidatorID: "published"},
	}
	for _, item := range valid {
		if err := ValidateValidator(item); err != nil {
			t.Fatalf("valid Validator rejected: %+v: %v", item, err)
		}
	}
	bad := []Validator{
		{ID: "Bad", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"},
		{ID: "ref", ValidatorID: "Bad"}, {ID: "ref", ValidatorID: "catalog", Kind: "non-null"},
		{ID: "bad", Kind: "unknown", Scope: "row", Severity: "error"},
		{ID: "bad", Kind: "non-null", Scope: "set", Severity: "error", Field: "x"},
		{ID: "bad", Kind: "non-null", Scope: "row", Severity: "fatal", Field: "x"},
		{ID: "bad", Kind: "non-null", Scope: "row", Severity: "error"},
		{ID: "bad", Kind: "equals-fields", Scope: "row", Severity: "error", Field: "x"},
		{ID: "bad", Kind: "sum-equals", Scope: "set", Severity: "error", OtherField: "y"},
		{ID: "bad", Kind: "row-count", Scope: "set", Severity: "error"},
		{ID: "bad", Kind: "row-count", Scope: "set", Severity: "error", Expected: ptrValue(investigation.NewStringValue("2"))},
		{ID: "bad", Kind: "row-count", Scope: "set", Severity: "error", Expected: ptrValue(investigation.NewIntegerValue("-1"))},
		{ID: "bad", Kind: "check", Scope: "row", Severity: "error", CheckID: "Bad"},
		{ID: "bad", Kind: "check", Scope: "row", Severity: "error", CheckID: "child", Bindings: []Binding{{Input: "bad name", FromColumn: "x"}}},
		{ID: "bad", Kind: "non-null", Scope: "row", Severity: "error", Field: "x", OnUnavailable: "ignore"},
	}
	for i, item := range bad {
		if err := ValidateValidator(item); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid Validator variant %d accepted: %+v", i, item)
		}
	}
	if err := ValidatePublishedValidator(PublishedValidator{ID: "catalog", Title: "Catalog", Validator: valid[0]}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateQueryValidators(QueryValidators{QueryID: "invoice", Validators: []Validator{valid[0], valid[2]}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateQueryValidators(QueryValidators{QueryID: "invoice", Validators: []Validator{valid[0], valid[0]}}); err == nil {
		t.Fatal("duplicate query-local Validator accepted")
	}
}

func ptrValue(value Value) *Value { return &value }

func TestInputBindingSchemaAndValueMapping(t *testing.T) {
	parent := []Input{{Name: "window", Type: investigation.ValueTypeInteger, Cardinality: "range"}, {Name: "ids", Type: investigation.ValueTypeInteger, Cardinality: "set"}}
	child := []Input{{Name: "start", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}, {Name: "ids", Type: investigation.ValueTypeInteger, Cardinality: "set", Required: true}}
	bindings := []Binding{{Input: "start", FromInput: "window", Part: "start"}, {Input: "ids", FromInput: "ids"}}
	if err := validateChildBindings(parent, child, bindings, false); err != nil {
		t.Fatal(err)
	}
	if err := validateLocalBindings([]Input{{Name: "start", Cardinality: "scalar", Required: true}}, []Binding{{Input: "start", FromColumn: "invoice_id"}}); err != nil {
		t.Fatal(err)
	}
	invalid := [][]Binding{
		{{Input: "missing", FromInput: "window"}},
		{{Input: "start", FromInput: "unknown"}},
		{{Input: "start", FromInput: "ids"}, {Input: "ids", FromInput: "ids"}},
		{{Input: "start", FromInput: "window", Part: "middle"}},
		{{Input: "start", FromColumn: "invoice_id"}},
		{{Input: "start", FromInput: "window", Part: "start"}, {Input: "start", FromInput: "window", Part: "end"}},
	}
	for i, bs := range invalid {
		if err := validateChildBindings(parent, child, bs, false); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid child bindings %d accepted: %v", i, err)
		}
	}
	if err := validateLocalBindings(child, []Binding{{Input: "start", FromColumn: "x"}}); err == nil {
		t.Fatal("required local set binding absent")
	}
	if err := validateLocalBindings([]Input{{Name: "ids", Cardinality: "set"}}, []Binding{{Input: "ids", FromColumn: "x"}}); err == nil {
		t.Fatal("row column bound to set input")
	}
	if err := validateBindings([]Binding{{Input: "x", FromInput: "x"}, {Input: "x", FromColumn: "x"}}); err == nil {
		t.Fatal("duplicate binding accepted")
	}
	if err := validateBindings([]Binding{{Input: "x", FromInput: "x", FromColumn: "x"}}); err == nil {
		t.Fatal("ambiguous binding accepted")
	}
	if validColumnName("bad/name") || validColumnName("bad\\name") || validColumnName("bad\nname") || !validColumnName("Invoice Id") {
		t.Fatal("column-name validation broken")
	}
	window := InputValue{Range: &ValueRange{Type: "range", Start: investigation.NewIntegerValue("1"), End: investigation.NewIntegerValue("10")}}
	ids := InputValue{Set: &apicontract.TypedValueSet{Values: []Value{investigation.NewIntegerValue("1"), investigation.NewIntegerValue("3")}}}
	mapped, err := mapBindings(bindings, map[string]InputValue{"window": window, "ids": ids}, nil, nil)
	if err != nil || mapped["start"].Scalar == nil || mapped["start"].Scalar.Str != "1" || mapped["ids"].Set == nil {
		t.Fatalf("range/set mapping: %+v %v", mapped, err)
	}
	mapped, err = mapBindings([]Binding{{Input: "end", FromInput: "window", Part: "end"}, {Input: "id", FromColumn: "invoice_id"}}, map[string]InputValue{"window": window}, []apicontract.Column{{Name: "invoice_id"}}, []Value{investigation.NewIntegerValue("9")})
	if err != nil || mapped["end"].Scalar.Str != "10" || mapped["id"].Scalar.Str != "9" {
		t.Fatalf("range end/row mapping: %+v %v", mapped, err)
	}
	for _, bs := range [][]Binding{{{Input: "x", FromInput: "missing"}}, {{Input: "x", FromInput: "ids", Part: "start"}}, {{Input: "x", FromColumn: "missing"}}} {
		if _, err := mapBindings(bs, map[string]InputValue{"ids": ids}, nil, nil); err == nil {
			t.Fatalf("unavailable binding accepted: %+v", bs)
		}
	}
	if err := ValidateInputs(child, mapped); err == nil {
		t.Fatal("extra mapped input accepted")
	}
	if err := ValidateInputs(child, map[string]InputValue{}); err == nil {
		t.Fatal("required inputs absent")
	}
	if err := ValidateInputs([]Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar"}}, map[string]InputValue{"id": {Scalar: ptrValue(investigation.NewStringValue("1"))}}); err == nil {
		t.Fatal("wrong scalar type accepted")
	}
	if err := ValidateInputs([]Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar"}}, map[string]InputValue{"id": {Scalar: ptrValue(investigation.NewIntegerValue("1"))}}); err != nil {
		t.Fatal(err)
	}
}

func TestOrderedRangeAndCountRejectInvalidEndpointsAndOverflow(t *testing.T) {
	for _, value := range []InputValue{
		{Range: &ValueRange{Type: "range", Start: investigation.NewIntegerValue("bad"), End: investigation.NewIntegerValue("2")}},
		{Range: &ValueRange{Type: "range", Start: investigation.NewIntegerValue("1"), End: investigation.NewIntegerValue("bad")}},
	} {
		if err := value.Validate(); err == nil {
			t.Fatalf("invalid range endpoint accepted: %+v", value)
		}
	}
	var value InputValue
	if err := json.Unmarshal([]byte(`{"type":7}`), &value); err == nil {
		t.Fatal("non-string InputValue discriminant accepted")
	}
	large := investigation.NewIntegerValue("999999999999999999999999999999999")
	if err := ValidateValidator(Validator{ID: "count", Kind: "row-count", Scope: "set", Severity: "error", Expected: &large}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overflowing row-count assertion accepted: %v", err)
	}
	malformed := investigation.NewIntegerValue("01")
	if err := ValidateValidator(Validator{ID: "count", Kind: "row-count", Scope: "set", Severity: "error", Expected: &malformed}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("noncanonical row-count assertion accepted: %v", err)
	}
	if err := validateChildBindings([]Input{{Name: "window", Type: investigation.ValueTypeDate, Cardinality: "range"}}, []Input{{Name: "start", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}, []Binding{{Input: "start", FromInput: "window", Part: "start"}}, false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("date range bound to integer input: %v", err)
	}
}
