package detective

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/investigation"
)

func TestBindCaseInputsPreservesCohortAndUnrelatedScalarFacts(t *testing.T) {
	one := investigation.NewIntegerValue("1")
	seven := investigation.NewIntegerValue("7")
	set := InputValue{Set: &apicontract.TypedValueSet{Values: []Value{one, seven}}}
	window := InputValue{Range: &ValueRange{Type: "range", Start: investigation.NewDateValue("2026-10-01"), End: investigation.NewDateValue("2026-10-07")}}
	status := InputValue{Scalar: ptrValue(investigation.NewStringValue("investigating"))}
	schema := []Input{
		{Name: "invoice_ids", Type: investigation.ValueTypeInteger, Cardinality: "set", Required: true},
		{Name: "invoice_dates", Type: investigation.ValueTypeDate, Cardinality: "range", Required: true},
		{Name: "source_id", Type: investigation.ValueTypeInteger, Cardinality: "scalar"},
	}
	context := map[string]InputValue{"invoice_ids": set, "invoice_dates": window, "status": status}
	supplied := map[string]InputValue{"source_id": {Scalar: &one}}
	got, err := BindCaseInputs(schema, supplied, context)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !reflect.DeepEqual(got["invoice_ids"], set) || !reflect.DeepEqual(got["invoice_dates"], window) || !reflect.DeepEqual(got["source_id"], supplied["source_id"]) {
		t.Fatalf("unexpected effective inputs: %+v", got)
	}
	if _, exists := got["status"]; exists {
		t.Fatal("unrelated scalar Case fact was passed as Check input")
	}
	if len(supplied) != 1 {
		t.Fatal("caller inputs were mutated")
	}
	if err := MatchCaseCohort(context, got); err != nil {
		t.Fatal(err)
	}
}

func TestBindCaseInputsRejectsReplacementOrUndeclaredCohorts(t *testing.T) {
	one := investigation.NewIntegerValue("1")
	two := investigation.NewIntegerValue("2")
	set := InputValue{Set: &apicontract.TypedValueSet{Values: []Value{one}}}
	otherSet := InputValue{Set: &apicontract.TypedValueSet{Values: []Value{two}}}
	window := InputValue{Range: &ValueRange{Type: "range", Start: investigation.NewDateValue("2026-10-01"), End: investigation.NewDateValue("2026-10-07")}}
	otherWindow := InputValue{Range: &ValueRange{Type: "range", Start: investigation.NewDateValue("2026-10-02"), End: investigation.NewDateValue("2026-10-07")}}
	schema := []Input{{Name: "invoice_ids", Type: investigation.ValueTypeInteger, Cardinality: "set", Required: true}, {Name: "invoice_dates", Type: investigation.ValueTypeDate, Cardinality: "range", Required: true}}
	context := map[string]InputValue{"invoice_ids": set, "invoice_dates": window}
	for _, supplied := range []map[string]InputValue{
		{"invoice_ids": otherSet},
		{"invoice_dates": otherWindow},
		{"invoice_ids": {Scalar: &one}},
		{"unknown": {Scalar: &one}},
	} {
		if _, err := BindCaseInputs(schema, supplied, context); !errors.Is(err, ErrInvalid) {
			t.Fatalf("replacement accepted: %+v %v", supplied, err)
		}
	}
	if _, err := BindCaseInputs(nil, nil, context); !errors.Is(err, ErrInvalid) {
		t.Fatalf("undeclared Case cohort accepted: %v", err)
	}
	if _, err := BindCaseInputs([]Input{{Name: "invoice_ids", Type: investigation.ValueTypeString, Cardinality: "scalar"}}, nil, map[string]InputValue{"invoice_ids": set}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Case set bound to incompatible schema: %v", err)
	}
	if _, err := BindCaseInputs([]Input{{Name: "invoice_ids", Type: investigation.ValueTypeInteger, Cardinality: "set", Required: true}}, nil, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing required cohort accepted: %v", err)
	}
	if _, err := BindCaseInputs(nil, nil, map[string]InputValue{"broken": {}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid Case context accepted: %v", err)
	}
	for _, invalid := range [][]Input{
		{{Name: "bad name", Type: investigation.ValueTypeInteger, Cardinality: "scalar"}},
		{{Name: "invoice_ids", Type: investigation.ValueTypeInteger, Cardinality: "set"}, {Name: "invoice_ids", Type: investigation.ValueTypeInteger, Cardinality: "set"}},
		{{Name: "invoice_ids", Type: "opaque", Cardinality: "set"}},
		{{Name: "invoice_ids", Type: investigation.ValueTypeInteger, Cardinality: "many"}},
	} {
		if _, err := BindCaseInputs(invalid, nil, nil); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid Check input schema accepted: %+v %v", invalid, err)
		}
	}
}

func TestBoundCaseInputsOwnAcceptedScalarSetAndRangeValues(t *testing.T) {
	for _, fromCase := range []bool{false, true} {
		name := "supplied"
		if fromCase {
			name = "case"
		}
		t.Run(name, func(t *testing.T) {
			original := map[string]InputValue{
				"id":     {Scalar: ptrValue(investigation.NewIntegerValue("1"))},
				"ids":    {Set: &apicontract.TypedValueSet{Values: []Value{investigation.NewIntegerValue("1")}}},
				"window": {Range: &ValueRange{Type: "range", Start: investigation.NewDateValue("2026-10-01"), End: investigation.NewDateValue("2026-10-07")}},
			}
			schema := []Input{
				{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar"},
				{Name: "ids", Type: investigation.ValueTypeInteger, Cardinality: "set"},
				{Name: "window", Type: investigation.ValueTypeDate, Cardinality: "range"},
			}
			var supplied, context map[string]InputValue
			if fromCase {
				context = original
			} else {
				supplied = original
			}
			got, err := BindCaseInputs(schema, supplied, context)
			if err != nil {
				t.Fatal(err)
			}
			got["id"].Scalar.Str = "99"
			got["ids"].Set.Values[0].Str = "99"
			got["window"].Range.Start.Str = "2026-10-02"
			if original["id"].Scalar.Str != "1" || original["ids"].Set.Values[0].Str != "1" || original["window"].Range.Start.Str != "2026-10-01" {
				t.Fatal("accepted inputs mutated caller-owned Case or supplied values")
			}
			original["id"].Scalar.Str = "2"
			original["ids"].Set.Values[0].Str = "2"
			original["window"].Range.End.Str = "2026-10-08"
			if got["id"].Scalar.Str != "99" || got["ids"].Set.Values[0].Str != "99" || got["window"].Range.End.Str != "2026-10-07" {
				t.Fatal("caller mutation changed accepted inputs")
			}
		})
	}
}

func TestMatchCaseCohortRequiresExactConfirmedValues(t *testing.T) {
	one := investigation.NewIntegerValue("1")
	two := investigation.NewIntegerValue("2")
	set := InputValue{Set: &apicontract.TypedValueSet{Values: []Value{one}}}
	otherSet := InputValue{Set: &apicontract.TypedValueSet{Values: []Value{two}}}
	window := InputValue{Range: &ValueRange{Type: "range", Start: investigation.NewDateValue("2026-10-01"), End: investigation.NewDateValue("2026-10-07")}}
	otherWindow := InputValue{Range: &ValueRange{Type: "range", Start: investigation.NewDateValue("2026-10-02"), End: investigation.NewDateValue("2026-10-07")}}
	context := map[string]InputValue{"invoice_ids": set, "invoice_dates": window, "reviewed_by": {Scalar: &one}}
	good := map[string]InputValue{"invoice_ids": set, "invoice_dates": window}
	if err := MatchCaseCohort(context, good); err != nil {
		t.Fatal(err)
	}
	for _, inputs := range []map[string]InputValue{
		{"invoice_dates": window},
		{"invoice_ids": set},
		{"invoice_ids": otherSet, "invoice_dates": window},
		{"invoice_ids": set, "invoice_dates": otherWindow},
		{"invoice_ids": set, "invoice_dates": window, "reviewed_by": {Scalar: &two}},
		{"invoice_ids": {}, "invoice_dates": window},
	} {
		if err := MatchCaseCohort(context, inputs); !errors.Is(err, ErrInvalid) {
			t.Fatalf("cohort conflict accepted: %+v %v", inputs, err)
		}
	}
	if err := MatchCaseCohort(map[string]InputValue{"broken": {}}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid Case context accepted: %v", err)
	}
}

func TestInputValueRangeRejectsUnknownAndDuplicateFields(t *testing.T) {
	valid := `{"type":"range","start":{"type":"date","value":"2026-10-01"},"end":{"type":"date","value":"2026-10-07"}}`
	for _, wire := range []string{
		`{"type":"range","start":{"type":"date","value":"2026-10-01"},"end":{"type":"date","value":"2026-10-07"},"scope":"foreign"}`,
		`{"type":"range","start":{"type":"date","value":"2026-10-01","scope":"foreign"},"end":{"type":"date","value":"2026-10-07"}}`,
		`{"type":"range","start":{"type":"date","value":"2026-10-01"},"start":{"type":"date","value":"2026-10-02"},"end":{"type":"date","value":"2026-10-07"}}`,
		`{"type":"range","start":{"type":"date","value":"2026-10-01","value":"2026-10-02"},"end":{"type":"date","value":"2026-10-07"}}`,
		`{"type":"range","Type":"range","start":{"type":"date","value":"2026-10-01"},"end":{"type":"date","value":"2026-10-07"}}`,
	} {
		var value InputValue
		if err := json.Unmarshal([]byte(wire), &value); err == nil {
			t.Fatalf("invalid range accepted: %s", wire)
		}
	}
	for _, wire := range []string{
		valid,
		`{"type":"integer","value":"7"}`,
		`{"type":"set","values":[{"type":"integer","value":"7"}]}`,
	} {
		var value InputValue
		if err := json.Unmarshal([]byte(wire), &value); err != nil {
			t.Fatalf("valid input rejected: %s: %v", wire, err)
		}
	}
}
