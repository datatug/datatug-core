package investigation

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidationHelpers(t *testing.T) {
	require.EqualError(t, &ValidationError{Field: "fact", Message: "is required"}, "fact: is required")
	require.EqualError(t, &ValidationError{Message: "invalid"}, "invalid")
	require.NoError(t, requireNonEmpty("fact", " value "))
	require.Error(t, requireNonEmpty("fact", " \t"))
	require.NoError(t, requireOneOf("role", "affected", "affected", "suspected"))
	require.Error(t, requireOneOf("role", "admin", "affected", "suspected"))
}

func TestTypedValueExactWireAndValidation(t *testing.T) {
	values := []struct {
		value TypedValue
		wire  string
	}{
		{NewStringValue("Rock"), `{"type":"string","value":"Rock"}`},
		{NewNumberValue(14.85), `{"type":"number","value":14.85}`},
		{NewIntegerValue("42"), `{"type":"integer","value":"42"}`},
		{NewDecimalValue("10.50"), `{"type":"decimal","value":"10.50"}`},
		{NewBooleanValue(false), `{"type":"boolean","value":false}`},
		{NewDateValue("2026-09-13"), `{"type":"date","value":"2026-09-13"}`},
		{NewDatetimeValue("2026-09-13T10:00:00Z"), `{"type":"datetime","value":"2026-09-13T10:00:00Z"}`},
		{NewNullValue(), `{"type":"null","value":null}`},
	}
	for _, test := range values {
		require.NoError(t, test.value.Validate())
		data, err := json.Marshal(test.value)
		require.NoError(t, err)
		require.JSONEq(t, test.wire, string(data))
		var decoded TypedValue
		require.NoError(t, json.Unmarshal(data, &decoded))
		require.Equal(t, test.value, decoded)
	}

	invalid := []TypedValue{
		NewNumberValue(math.Inf(1)), NewNumberValue(math.NaN()),
		NewIntegerValue("007"), NewDecimalValue("1e2"), NewDateValue("2026-02-30"),
		NewDatetimeValue("not-a-date"), NewDatetimeValue("2026-09-13T10:00:00+01:00"),
		{Type: "currency", Str: "5"},
	}
	for _, value := range invalid {
		require.Error(t, value.Validate())
	}
	_, err := json.Marshal(TypedValue{Type: "currency"})
	require.Error(t, err)
}

func TestTypedValueStrictDecodeFailures(t *testing.T) {
	invalid := []string{
		``, `"scalar"`, `[]`, `{"type"`, `{"type":`,
		`{"type":"string","value":"a","value":"b"}`,
		`{"type":"string","value":"a","extra":1}`,
		`{"value":"a"}`, `{"type":"string"}`,
		`{"type":1,"value":"a"}`, `{"type":"currency","value":"a"}`,
		`{"type":"string","value":1}`, `{"type":"number","value":"1"}`,
		`{"type":"boolean","value":"true"}`, `{"type":"null","value":"null"}`,
		`{"type":"integer","value":"007"}`,
	}
	for _, wire := range invalid {
		var value TypedValue
		require.Error(t, json.Unmarshal([]byte(wire), &value), wire)
	}
	var reordered TypedValue
	require.NoError(t, json.Unmarshal([]byte(`{"value":"Rock","type":"string"}`), &reordered))
	require.Equal(t, NewStringValue("Rock"), reordered)
	require.Error(t, checkTypedValueKeys(nil))
	require.Error(t, checkTypedValueKeys([]byte(`{"`)))
	require.Error(t, checkTypedValueKeys([]byte(`{"type":`)))
}

func TestPhysicalFactAndContextValidation(t *testing.T) {
	physical := PhysicalRef{Source: "crm", Collection: "Customer", Column: "ID"}
	require.NoError(t, physical.Validate())
	for _, invalid := range []PhysicalRef{
		{Collection: "Customer", Column: "ID"},
		{Source: "crm", Column: "ID"},
		{Source: "crm", Collection: "Customer"},
	} {
		require.Error(t, invalid.Validate())
	}

	valid := Fact{
		ID: "customer-id", Entity: "Customer", Field: "ID", Value: NewIntegerValue("5"),
		Origin: FactOriginSelection, Physical: &physical, Mapping: FactMappingDeclared,
		Enabled: true, Role: FactRoleAffected, Layer: "canonical",
	}
	require.NoError(t, valid.Validate())
	for _, role := range []string{FactRoleHealthyControl, FactRoleSuspected, FactRoleExcluded, FactRoleRecovered} {
		candidate := valid
		candidate.Role = role
		require.NoError(t, candidate.Validate())
	}
	for _, layer := range []string{"hypothesis:H1", "participant:alex", "question:q1"} {
		candidate := valid
		candidate.Layer = layer
		require.NoError(t, candidate.Validate())
	}
	require.False(t, validFactLayer("admin"))

	mutations := []func(*Fact){
		func(f *Fact) { f.ID = "" }, func(f *Fact) { f.Entity = "" }, func(f *Fact) { f.Field = "" },
		func(f *Fact) { f.Value = NewIntegerValue("05") }, func(f *Fact) { f.Origin = "derived" },
		func(f *Fact) { f.Physical = &PhysicalRef{Source: "crm"} }, func(f *Fact) { f.Mapping = "guessed" },
		func(f *Fact) { f.Role = "administrator" }, func(f *Fact) { f.Layer = "hypothesis:" },
	}
	for _, mutate := range mutations {
		candidate := valid
		mutate(&candidate)
		require.Error(t, candidate.Validate())
	}

	minimal := valid
	minimal.Physical, minimal.Mapping, minimal.Role, minimal.Layer = nil, "", "", ""
	minimal.Origin = FactOriginManual
	require.NoError(t, minimal.Validate())
	minimal.Origin = FactOriginContext
	require.NoError(t, minimal.Validate())

	require.NoError(t, (Context{Facts: []Fact{valid}}).Validate())
	require.Error(t, (Context{Facts: []Fact{{}}}).Validate())
	require.Error(t, (Context{Facts: []Fact{valid, valid}}).Validate())
}

func TestPolicyFactViews(t *testing.T) {
	fact := Fact{
		ID: "customer-email", Entity: "Customer", Field: "Email", Value: NewStringValue("secret@example.com"),
		Origin: FactOriginContext, Physical: &PhysicalRef{Source: "crm", Collection: "Customer", Column: "Email"},
		Mapping: FactMappingInferred, Enabled: true, Role: FactRoleAffected, Layer: "canonical",
	}
	visible := VisibleFact(fact)
	require.False(t, visible.Redacted())
	require.NoError(t, visible.Validate())
	visibleJSON, err := json.Marshal(visible)
	require.NoError(t, err)
	factJSON, err := json.Marshal(fact)
	require.NoError(t, err)
	require.JSONEq(t, string(factJSON), string(visibleJSON))

	redacted := RedactedFact(fact, true)
	require.True(t, redacted.Redacted())
	require.NoError(t, redacted.Validate())
	redactedJSON, err := json.Marshal(redacted)
	require.NoError(t, err)
	require.Contains(t, string(redactedJSON), `"value":{"redacted":true}`)
	require.NotContains(t, string(redactedJSON), "physical")
	withoutField := RedactedFact(fact, false)
	require.Empty(t, withoutField.Field)
	require.NoError(t, withoutField.Validate())

	for _, value := range []ValueView{VisibleValue(NewIntegerValue("5")), RedactedValue()} {
		require.NoError(t, value.Validate())
		data, marshalErr := json.Marshal(value)
		require.NoError(t, marshalErr)
		var decoded ValueView
		require.NoError(t, json.Unmarshal(data, &decoded))
		require.Equal(t, value.Redacted, decoded.Redacted)
		if value.Value != nil {
			require.Equal(t, *value.Value, *decoded.Value)
		}
	}

	for _, invalid := range []ValueView{{}, {Value: valuePointer(NewStringValue("x")), Redacted: true}, {Value: valuePointer(NewIntegerValue("01"))}} {
		require.Error(t, invalid.Validate())
		_, marshalErr := json.Marshal(invalid)
		require.Error(t, marshalErr)
	}
	for _, wire := range []string{``, `[]`, `{"redacted":false}`, `{"redacted":true,"type":"string"}`, `{"type":"integer","value":"01"}`} {
		var value ValueView
		require.Error(t, json.Unmarshal([]byte(wire), &value), wire)
	}

	invalidViews := []FactView{
		{},
		{ID: "f", Value: RedactedValue(), Origin: FactOriginContext},
		{ID: "f", Entity: "Customer", Value: VisibleValue(NewStringValue("x")), Origin: FactOriginContext},
		{ID: "f", Entity: "Customer", Field: "ID", Value: VisibleValue(NewIntegerValue("01")), Origin: FactOriginContext},
		{ID: "f", Entity: "Customer", Value: RedactedValue(), Origin: "derived"},
		{ID: "f", Entity: "Customer", Value: RedactedValue(), Origin: FactOriginContext, Physical: &PhysicalRef{Source: "crm"}},
		{ID: "f", Entity: "Customer", Value: RedactedValue(), Origin: FactOriginContext, Mapping: FactMappingDeclared},
		{ID: "f", Entity: "Customer", Value: RedactedValue(), Origin: FactOriginContext, Role: "admin"},
		{ID: "f", Entity: "Customer", Value: RedactedValue(), Origin: FactOriginContext, Layer: "question:"},
	}
	for _, view := range invalidViews {
		require.Error(t, view.Validate())
	}

	context := Context{Facts: []Fact{fact}}
	contextView := VisibleContext(context)
	require.NoError(t, contextView.Validate())
	require.Error(t, (ContextView{Facts: []FactView{{}}}).Validate())
	require.Error(t, (ContextView{Facts: []FactView{redacted, redacted}}).Validate())
}

func valuePointer(value TypedValue) *TypedValue { return &value }

func TestValidationErrorIdentity(t *testing.T) {
	err := NewIntegerValue("01").Validate()
	var validationError *ValidationError
	require.True(t, errors.As(err, &validationError))
}
