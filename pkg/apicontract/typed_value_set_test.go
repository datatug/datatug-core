package apicontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func mustTypedValueSet(t *testing.T, values ...TypedValue) TypedValueSet {
	t.Helper()
	set, err := NewTypedValueSet(values...)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestTypedValueSet_CanonicalizesAndRoundTrips(t *testing.T) {
	set := mustTypedValueSet(t, NewIntegerValue("2"), NewIntegerValue("1"), NewIntegerValue("2"))
	want := []TypedValue{NewIntegerValue("1"), NewIntegerValue("2")}
	if !reflect.DeepEqual(set.Values, want) {
		t.Fatalf("values = %#v, want %#v", set.Values, want)
	}
	data, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"type":"set","values":[{"type":"integer","value":"1"},{"type":"integer","value":"2"}]}` {
		t.Fatalf("unexpected wire value: %s", data)
	}
	var roundTrip TypedValueSet
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(roundTrip, set) {
		t.Fatalf("round trip = %#v, want %#v", roundTrip, set)
	}
}

func TestTypedValueSet_RejectsInvalidSets(t *testing.T) {
	oversized := make([]TypedValue, 501)
	for i := range oversized {
		oversized[i] = NewIntegerValue("1")
	}
	for name, values := range map[string][]TypedValue{
		"empty":     nil,
		"oversized": oversized,
		"null":      {NewNullValue()},
		"mixed":     {NewIntegerValue("1"), NewStringValue("2")},
		"invalid":   {NewIntegerValue("01")},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewTypedValueSet(values...); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	unsorted := TypedValueSet{Values: []TypedValue{NewIntegerValue("2"), NewIntegerValue("1")}}
	if err := unsorted.Validate(); err == nil {
		t.Fatal("uncanonical order should be rejected")
	}
	duplicate := TypedValueSet{Values: []TypedValue{NewIntegerValue("1"), NewIntegerValue("1")}}
	if err := duplicate.Validate(); err == nil {
		t.Fatal("duplicate canonical value should be rejected")
	}
}

func TestTypedValueOrSet_StrictUnion(t *testing.T) {
	set := mustTypedValueSet(t, NewStringValue("a"), NewStringValue("b"))
	for _, value := range []TypedValueOrSet{ScalarValue(NewBooleanValue(false)), SetValue(set)} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var got TypedValueOrSet
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, value) {
			t.Fatalf("round trip = %#v, want %#v", got, value)
		}
	}
	if err := (TypedValueOrSet{}).Validate(); err == nil {
		t.Fatal("empty union should be rejected")
	}
	both := TypedValueOrSet{Scalar: valuePointer(NewStringValue("a")), Set: &set}
	if err := both.Validate(); err == nil {
		t.Fatal("conflicting union should be rejected")
	}
	for _, body := range []string{
		`{"type":"set","values":[],"value":"legacy"}`,
		`{"type":"set","values":[{"type":"integer","value":"1"}],"values":[]}`,
	} {
		var value TypedValueOrSet
		if err := json.Unmarshal([]byte(body), &value); err == nil {
			t.Fatalf("invalid union accepted: %s", body)
		}
	}
}

func TestBinding_SetProvenanceGroups(t *testing.T) {
	set := mustTypedValueSet(t, NewIntegerValue("1"), NewIntegerValue("2"))
	valid := Binding{
		ParameterID: "CustomerIds", Value: SetValue(set), Origin: BindingOriginSelection,
		OriginEvidence: BindingOriginEvidenceClientReported,
		ValueFactIDs:   [][]string{{"fact-1", "fact-2"}, {"fact-3"}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid grouped provenance rejected: %v", err)
	}
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var got Binding
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, valid) {
		t.Fatalf("round trip = %#v, want %#v", got, valid)
	}
	for name, mutate := range map[string]func(*Binding){
		"scalar and groups":  func(b *Binding) { b.Value = ScalarValue(NewIntegerValue("1")) },
		"fact id and groups": func(b *Binding) { b.FactID = "fact-1" },
		"group count":        func(b *Binding) { b.ValueFactIDs = b.ValueFactIDs[:1] },
		"empty group":        func(b *Binding) { b.ValueFactIDs[0] = nil },
		"unsorted group":     func(b *Binding) { b.ValueFactIDs[0] = []string{"fact-2", "fact-1"} },
		"fact in two groups": func(b *Binding) { b.ValueFactIDs[1] = []string{"fact-1"} },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.ValueFactIDs = make([][]string, len(valid.ValueFactIDs))
			for i := range valid.ValueFactIDs {
				candidate.ValueFactIDs[i] = append([]string(nil), valid.ValueFactIDs[i]...)
			}
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	manual := valid
	manual.Origin, manual.ValueFactIDs = BindingOriginManual, nil
	if err := manual.Validate(); err != nil {
		t.Fatalf("manual set without provenance rejected: %v", err)
	}
}
