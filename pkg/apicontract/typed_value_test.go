package apicontract

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestTypedValue_MarshalUnmarshalRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		value TypedValue
		wire  string
	}{
		{"string", NewStringValue("Rock"), `{"type":"string","value":"Rock"}`},
		{"number", NewNumberValue(14.85), `{"type":"number","value":14.85}`},
		{"number zero", NewNumberValue(0), `{"type":"number","value":0}`},
		{"integer", NewIntegerValue("42"), `{"type":"integer","value":"42"}`},
		{"integer negative", NewIntegerValue("-42"), `{"type":"integer","value":"-42"}`},
		{"integer zero", NewIntegerValue("0"), `{"type":"integer","value":"0"}`},
		{"integer large", NewIntegerValue("90071992547409925"), `{"type":"integer","value":"90071992547409925"}`},
		{"decimal", NewDecimalValue("10.50"), `{"type":"decimal","value":"10.50"}`},
		{"boolean true", NewBooleanValue(true), `{"type":"boolean","value":true}`},
		{"boolean false", NewBooleanValue(false), `{"type":"boolean","value":false}`},
		{"date", NewDateValue("2026-09-09"), `{"type":"date","value":"2026-09-09"}`},
		{"datetime", NewDatetimeValue("2026-09-09T12:00:00Z"), `{"type":"datetime","value":"2026-09-09T12:00:00Z"}`},
		{"null", NewNullValue(), `{"type":"null","value":null}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := json.Marshal(c.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(data) != c.wire {
				t.Errorf("marshal: got %s, want %s", data, c.wire)
			}
			var back TypedValue
			if err := json.Unmarshal(data, &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if back != c.value {
				t.Errorf("round-trip: got %+v, want %+v", back, c.value)
			}
			// And unmarshal must accept the wire form directly (not just our
			// own marshaled output), so field order in a real client's JSON
			// (value before type, say) still works.
			var direct TypedValue
			if err := json.Unmarshal([]byte(c.wire), &direct); err != nil {
				t.Fatalf("unmarshal wire form: %v", err)
			}
			if direct != c.value {
				t.Errorf("unmarshal wire form: got %+v, want %+v", direct, c.value)
			}
		})
	}
}

func TestTypedValue_UnmarshalFieldOrderIndependent(t *testing.T) {
	var v TypedValue
	if err := json.Unmarshal([]byte(`{"value":"Rock","type":"string"}`), &v); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != NewStringValue("Rock") {
		t.Errorf("got %+v", v)
	}
}

func TestTypedValue_UnmarshalRejectsUnknownField(t *testing.T) {
	var v TypedValue
	err := json.Unmarshal([]byte(`{"type":"string","value":"Rock","extra":1}`), &v)
	if err == nil {
		t.Fatal("expected an error for an unknown field")
	}
}

func TestTypedValue_UnmarshalRejectsDuplicateKey(t *testing.T) {
	var v TypedValue
	err := json.Unmarshal([]byte(`{"type":"string","value":"a","value":"b"}`), &v)
	if err == nil {
		t.Fatal("expected an error for a duplicate key")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected a duplicate-key error, got: %v", err)
	}
}

func TestTypedValue_UnmarshalRejectsMissingType(t *testing.T) {
	var v TypedValue
	if err := json.Unmarshal([]byte(`{"value":"Rock"}`), &v); err == nil {
		t.Fatal("expected an error: missing type")
	}
}

func TestTypedValue_UnmarshalRejectsMissingValue(t *testing.T) {
	var v TypedValue
	if err := json.Unmarshal([]byte(`{"type":"string"}`), &v); err == nil {
		t.Fatal("expected an error: missing value")
	}
}

func TestTypedValue_UnmarshalRejectsUnknownType(t *testing.T) {
	var v TypedValue
	if err := json.Unmarshal([]byte(`{"type":"currency","value":"5"}`), &v); err == nil {
		t.Fatal("expected an error: unknown type discriminant")
	}
}

func TestTypedValue_UnmarshalRejectsWrongJSONShapeForType(t *testing.T) {
	cases := []struct {
		name string
		wire string
	}{
		{"string value is a number", `{"type":"string","value":5}`},
		{"number value is a string", `{"type":"number","value":"5"}`},
		{"integer value is a number, not a string", `{"type":"integer","value":5}`},
		{"boolean value is a string", `{"type":"boolean","value":"true"}`},
		{"null value is not JSON null", `{"type":"null","value":"null"}`},
		{"date value is a number", `{"type":"date","value":20260909}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var v TypedValue
			if err := json.Unmarshal([]byte(c.wire), &v); err == nil {
				t.Errorf("expected an error for %s", c.wire)
			}
		})
	}
}

func TestTypedValue_UnmarshalRejectsInvalidContent(t *testing.T) {
	// UnmarshalJSON calls Validate() internally, so a syntactically-fine but
	// semantically invalid value (e.g. a non-canonical integer) is refused at
	// decode time, not just by a caller remembering to call Validate() later.
	var v TypedValue
	if err := json.Unmarshal([]byte(`{"type":"integer","value":"007"}`), &v); err == nil {
		t.Fatal("expected an error: non-canonical integer")
	}
}

func TestTypedValue_Validate_Number(t *testing.T) {
	if err := NewNumberValue(3.14).Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
	if err := NewNumberValue(0).Validate(); err != nil {
		t.Errorf("expected zero to be a valid number, got: %v", err)
	}
	if err := NewNumberValue(math.Inf(1)).Validate(); err == nil {
		t.Error("expected an error: +Inf is not finite")
	}
	if err := NewNumberValue(math.NaN()).Validate(); err == nil {
		t.Error("expected an error: NaN is not finite")
	}
}

func TestTypedValue_Validate_Integer(t *testing.T) {
	valid := []string{"0", "-1", "42", "-42", "90071992547409925"}
	for _, s := range valid {
		if err := NewIntegerValue(s).Validate(); err != nil {
			t.Errorf("expected %q to be a valid canonical integer, got: %v", s, err)
		}
	}
	invalid := []string{"", "007", "+5", "-0", "1.5", "abc", " 5", "5 ", "-"}
	for _, s := range invalid {
		if err := NewIntegerValue(s).Validate(); err == nil {
			t.Errorf("expected %q to be rejected as a non-canonical integer", s)
		}
	}
}

func TestTypedValue_Validate_Decimal(t *testing.T) {
	valid := []string{"0", "0.5", "-0.5", "10.50", "-42", "123.456789"}
	for _, s := range valid {
		if err := NewDecimalValue(s).Validate(); err != nil {
			t.Errorf("expected %q to be a valid canonical decimal, got: %v", s, err)
		}
	}
	invalid := []string{"", "007", "+5", "5.", ".5", "1e5", "abc", "5,0"}
	for _, s := range invalid {
		if err := NewDecimalValue(s).Validate(); err == nil {
			t.Errorf("expected %q to be rejected as a non-canonical decimal", s)
		}
	}
}

func TestTypedValue_Validate_Date(t *testing.T) {
	if err := NewDateValue("2026-09-09").Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
	invalid := []string{"", "2026-02-30", "09/09/2026", "2026-9-9", "not-a-date", "2026-13-01"}
	for _, s := range invalid {
		if err := NewDateValue(s).Validate(); err == nil {
			t.Errorf("expected %q to be rejected as an invalid calendar date", s)
		}
	}
}

func TestTypedValue_Validate_Datetime(t *testing.T) {
	if err := NewDatetimeValue("2026-09-09T12:00:00Z").Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
	invalid := []string{
		"",
		"2026-09-09",                // not a datetime
		"2026-09-09T12:00:00+01:00", // valid RFC3339 but not normalized to UTC
		"2026-09-09T12:00:00",       // missing offset
		"not-a-datetime",
	}
	for _, s := range invalid {
		if err := NewDatetimeValue(s).Validate(); err == nil {
			t.Errorf("expected %q to be rejected as a non-UTC-normalized datetime", s)
		}
	}
}

func TestTypedValue_MarshalJSON_UnknownType(t *testing.T) {
	v := TypedValue{Type: "currency", Str: "5"}
	if _, err := json.Marshal(v); err == nil {
		t.Fatal("expected an error marshaling an unknown TypedValue type")
	}
}

func TestTypedValue_UnmarshalRejectsNonStringType(t *testing.T) {
	var v TypedValue
	if err := json.Unmarshal([]byte(`{"type":123,"value":"x"}`), &v); err == nil {
		t.Fatal("expected an error: type must be a JSON string")
	}
}

func TestTypedValue_UnmarshalRejectsNonObjectTopLevel(t *testing.T) {
	// Syntactically valid JSON (so the duplicate-key pre-pass accepts it) but
	// not an object, so unmarshaling into map[string]json.RawMessage fails.
	var v TypedValue
	if err := json.Unmarshal([]byte(`[1,2,3]`), &v); err == nil {
		t.Fatal("expected an error: a JSON array is not a valid TypedValue")
	}
}

func TestTypedValue_UnmarshalRejectsMalformedJSON(t *testing.T) {
	var v TypedValue
	if err := json.Unmarshal([]byte(`{"type":`), &v); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestTypedValue_Validate_UnknownType(t *testing.T) {
	v := TypedValue{Type: "currency", Str: "5"}
	if err := v.Validate(); err == nil {
		t.Fatal("expected an error for an unknown type")
	}
}

func TestTypedValue_Validate_StringBooleanNullAlwaysValid(t *testing.T) {
	if err := NewStringValue("").Validate(); err != nil {
		t.Errorf("empty string must be a valid present value, got: %v", err)
	}
	if err := NewBooleanValue(false).Validate(); err != nil {
		t.Errorf("false must be a valid present value, got: %v", err)
	}
	if err := NewNullValue().Validate(); err != nil {
		t.Errorf("null must be valid, got: %v", err)
	}
}
