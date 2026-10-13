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

func TestMatchScenarioQueryCohortReconstructsExactClosedRange(t *testing.T) {
	start := investigation.NewDateValue("2026-10-01")
	end := investigation.NewDateValue("2026-10-07")
	id := investigation.NewIntegerValue("7")
	status := investigation.NewStringValue("investigating")
	ids := InputValue{Set: &apicontract.TypedValueSet{Values: []Value{id}}}
	window := InputValue{Range: &ValueRange{Type: "range", Start: start, End: end}}
	context := map[string]InputValue{
		"window": window,
		"ids":    ids,
		"status": {Scalar: &status},
	}
	step := ScenarioStep{Kind: "query", Bindings: []Binding{
		{Input: "from_date", FromInput: "window", Part: "start"},
		{Input: "through_date", FromInput: "window", Part: "end"},
		{Input: "invoice_ids", FromInput: "ids"},
		{Input: "case_status", FromInput: "status"},
	}}
	params := map[string]InputValue{
		"from_date":    {Scalar: ptrValue(start)},
		"through_date": {Scalar: ptrValue(end)},
		"invoice_ids":  ids,
		"case_status":  {Scalar: ptrValue(status)},
	}
	contextBefore, paramsBefore := map[string]InputValue{}, map[string]InputValue{}
	for name, value := range context {
		contextBefore[name] = value
	}
	for name, value := range params {
		paramsBefore[name] = value
	}
	if err := MatchScenarioQueryCohort(step, context, params); err != nil {
		t.Fatalf("matching recorded query bounds rejected: %v", err)
	}
	if !reflect.DeepEqual(context, contextBefore) || !reflect.DeepEqual(params, paramsBefore) {
		t.Fatal("cohort matching mutated caller inputs")
	}

	changed := investigation.NewDateValue("2026-10-02")
	for name, replacement := range map[string]Value{"from_date": changed, "through_date": changed} {
		mismatch := map[string]InputValue{}
		for key, value := range params {
			mismatch[key] = value
		}
		mismatch[name] = InputValue{Scalar: ptrValue(replacement)}
		if err := MatchScenarioQueryCohort(step, context, mismatch); !errors.Is(err, ErrInvalid) {
			t.Errorf("changed %s bound accepted: %v", name, err)
		}
	}

	invalid := []struct {
		name   string
		step   ScenarioStep
		params map[string]InputValue
	}{
		{"missing start", step, map[string]InputValue{"through_date": params["through_date"], "invoice_ids": ids}},
		{"missing end", step, map[string]InputValue{"from_date": params["from_date"], "invoice_ids": ids}},
		{"duplicate start", ScenarioStep{Kind: "query", Bindings: append(append([]Binding(nil), step.Bindings...), Binding{Input: "another_start", FromInput: "window", Part: "start"})}, map[string]InputValue{"from_date": params["from_date"], "through_date": params["through_date"], "another_start": params["from_date"], "invoice_ids": ids, "case_status": params["case_status"]}},
		{"duplicate query parameter", ScenarioStep{Kind: "query", Bindings: append(append([]Binding(nil), step.Bindings...), Binding{Input: "from_date", FromInput: "window", Part: "end"})}, params},
		{"wrong bound type", step, map[string]InputValue{"from_date": {Scalar: ptrValue(investigation.NewIntegerValue("1"))}, "through_date": params["through_date"], "invoice_ids": ids}},
		{"range bound is not scalar", step, map[string]InputValue{"from_date": ids, "through_date": params["through_date"], "invoice_ids": ids}},
		{"query parameter contains range", step, map[string]InputValue{"from_date": window, "through_date": params["through_date"], "invoice_ids": ids}},
		{"range used as a whole", ScenarioStep{Kind: "query", Bindings: []Binding{{Input: "window", FromInput: "window"}}}, map[string]InputValue{"window": window}},
		{"range binding omits part", ScenarioStep{Kind: "query", Bindings: []Binding{{Input: "window", FromInput: "window"}}}, map[string]InputValue{"window": {Scalar: ptrValue(start)}}},
		{"column binding", ScenarioStep{Kind: "query", Bindings: []Binding{{Input: "from_date", FromColumn: "created_at"}}}, params},
		{"wrong step kind", ScenarioStep{Kind: "check"}, params},
		{"scalar source cannot select a part", ScenarioStep{Kind: "query", Bindings: []Binding{{Input: "status_param", FromInput: "status", Part: "start"}}}, map[string]InputValue{"status_param": params["case_status"]}},
		{"conflicting duplicate source", ScenarioStep{Kind: "query", Bindings: append(append([]Binding(nil), step.Bindings...), Binding{Input: "other_status", FromInput: "status"})}, map[string]InputValue{"from_date": params["from_date"], "through_date": params["through_date"], "invoice_ids": ids, "case_status": params["case_status"], "other_status": {Scalar: ptrValue(investigation.NewStringValue("closed"))}}},
		{"missing set cohort", ScenarioStep{Kind: "query", Bindings: []Binding{{Input: "from_date", FromInput: "window", Part: "start"}, {Input: "through_date", FromInput: "window", Part: "end"}}}, map[string]InputValue{"from_date": params["from_date"], "through_date": params["through_date"]}},
		{"changed set cohort", step, map[string]InputValue{"from_date": params["from_date"], "through_date": params["through_date"], "invoice_ids": {Set: &apicontract.TypedValueSet{Values: []Value{investigation.NewIntegerValue("8")}}}, "case_status": params["case_status"]}},
		{"changed scalar cohort", step, map[string]InputValue{"from_date": params["from_date"], "through_date": params["through_date"], "invoice_ids": ids, "case_status": {Scalar: ptrValue(investigation.NewStringValue("closed"))}}},
		{"set cardinality mismatch", step, map[string]InputValue{"from_date": params["from_date"], "through_date": params["through_date"], "invoice_ids": {Scalar: ptrValue(id)}, "case_status": params["case_status"]}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if err := MatchScenarioQueryCohort(test.step, context, test.params); !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid query cohort accepted: %v", err)
			}
		})
	}
	if err := MatchScenarioQueryCohort(step, context, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing recorded bound accepted: %v", err)
	}
	if err := MatchScenarioQueryCohort(ScenarioStep{Kind: "query"}, context, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbound Case range accepted: %v", err)
	}
	if err := MatchScenarioQueryCohort(ScenarioStep{Kind: "query", Bindings: []Binding{{Input: "a", FromInput: "free", Part: "start"}, {Input: "b", FromInput: "free", Part: "end"}}}, nil, map[string]InputValue{
		"a": {Scalar: ptrValue(start)}, "b": {Scalar: ptrValue(end)},
	}); err != nil {
		t.Fatalf("well-formed non-Case range input should not become a Case cohort: %v", err)
	}
	for _, params := range []map[string]InputValue{
		{"a": {Scalar: ptrValue(start)}},
		{"a": {Scalar: ptrValue(start)}, "b": {Scalar: ptrValue(investigation.NewIntegerValue("7"))}},
		{"a": ids, "b": {Scalar: ptrValue(end)}},
		{"a": {Scalar: ptrValue(investigation.NewStringValue("not a range bound"))}, "b": {Scalar: ptrValue(investigation.NewStringValue("not a range bound"))}},
		{"a": {Scalar: ptrValue(end)}, "b": {Scalar: ptrValue(start)}},
	} {
		step := ScenarioStep{Kind: "query", Bindings: []Binding{{Input: "a", FromInput: "free", Part: "start"}, {Input: "b", FromInput: "free", Part: "end"}}}
		if err := MatchScenarioQueryCohort(step, nil, params); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid unmapped range accepted: %+v %v", params, err)
		}
	}
	duplicateUnknown := ScenarioStep{Kind: "query", Bindings: []Binding{{Input: "a", FromInput: "free", Part: "start"}, {Input: "b", FromInput: "free", Part: "end"}, {Input: "c", FromInput: "free", Part: "start"}}}
	if err := MatchScenarioQueryCohort(duplicateUnknown, nil, map[string]InputValue{"a": {Scalar: ptrValue(start)}, "b": {Scalar: ptrValue(end)}, "c": {Scalar: ptrValue(start)}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate unmapped range endpoint accepted: %v", err)
	}
	statusOnly := map[string]InputValue{"status": {Scalar: ptrValue(status)}}
	if err := MatchScenarioQueryCohort(ScenarioStep{Kind: "query"}, statusOnly, statusOnly); err != nil {
		t.Fatalf("directly recorded scalar Case value rejected: %v", err)
	}
	if err := MatchScenarioQueryCohort(ScenarioStep{Kind: "query"}, statusOnly, map[string]InputValue{"status": {}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid directly recorded scalar accepted: %v", err)
	}
	if err := MatchScenarioQueryCohort(ScenarioStep{Kind: "query"}, map[string]InputValue{"broken": {}}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid Case value accepted: %v", err)
	}
	if err := MatchScenarioQueryCohort(ScenarioStep{Kind: "query", Bindings: []Binding{{Input: "broken", FromInput: "broken"}}}, map[string]InputValue{"broken": {}}, map[string]InputValue{"broken": {Scalar: ptrValue(status)}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid bound Case value accepted: %v", err)
	}
}

func TestScenarioRangePartInsertionRejectsDuplicateAndInvalidPart(t *testing.T) {
	parts := map[string]*scenarioRangeParts{}
	date := investigation.NewDateValue("2026-10-01")
	if err := addScenarioRangePart(parts, "window", "start", date); err != nil {
		t.Fatal(err)
	}
	if err := addScenarioRangePart(parts, "window", "start", date); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate range start accepted: %v", err)
	}
	if err := addScenarioRangePart(parts, "window", "end", date); err != nil {
		t.Fatal(err)
	}
	if err := addScenarioRangePart(parts, "window", "end", date); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate range end accepted: %v", err)
	}
	if err := addScenarioRangePart(parts, "window", "other", date); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid range part accepted: %v", err)
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
