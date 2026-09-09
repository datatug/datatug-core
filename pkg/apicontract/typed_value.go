package apicontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

// ValueType is TypedValue's tagged-union discriminant - the closed set of
// "type" values api-contract.md's TypedValue union declares.
type ValueType string

const (
	ValueTypeString   ValueType = "string"
	ValueTypeNumber   ValueType = "number"
	ValueTypeInteger  ValueType = "integer"
	ValueTypeDecimal  ValueType = "decimal"
	ValueTypeBoolean  ValueType = "boolean"
	ValueTypeDate     ValueType = "date"
	ValueTypeDatetime ValueType = "datetime"
	ValueTypeNull     ValueType = "null"
)

// TypedValue is the wire tagged union every value crossing the transport
// boundary uses - api-contract.md "Shared JSON types". Missing and null
// differ (omit an unbound parameter; null satisfies only a nullable
// parameter); false, zero and empty string are present values, never
// coerced. Exactly one of Str/Num/Bool holds the payload, selected by Type:
// Str for string/integer/decimal/date/datetime (the wire encodes all of
// these as a JSON string, per the union), Num for number (a JSON number),
// Bool for boolean (a JSON boolean); null carries no payload.
type TypedValue struct {
	Type ValueType
	Str  string
	Num  float64
	Bool bool
}

func NewStringValue(v string) TypedValue   { return TypedValue{Type: ValueTypeString, Str: v} }
func NewNumberValue(v float64) TypedValue  { return TypedValue{Type: ValueTypeNumber, Num: v} }
func NewIntegerValue(v string) TypedValue  { return TypedValue{Type: ValueTypeInteger, Str: v} }
func NewDecimalValue(v string) TypedValue  { return TypedValue{Type: ValueTypeDecimal, Str: v} }
func NewBooleanValue(v bool) TypedValue    { return TypedValue{Type: ValueTypeBoolean, Bool: v} }
func NewDateValue(v string) TypedValue     { return TypedValue{Type: ValueTypeDate, Str: v} }
func NewDatetimeValue(v string) TypedValue { return TypedValue{Type: ValueTypeDatetime, Str: v} }
func NewNullValue() TypedValue             { return TypedValue{Type: ValueTypeNull} }

// MarshalJSON writes the exact `{type:'...', value:...}` wire shape, with
// value's own JSON kind (string/number/boolean/null) selected by Type.
func (v TypedValue) MarshalJSON() ([]byte, error) {
	switch v.Type {
	case ValueTypeString, ValueTypeInteger, ValueTypeDecimal, ValueTypeDate, ValueTypeDatetime:
		return json.Marshal(struct {
			Type  ValueType `json:"type"`
			Value string    `json:"value"`
		}{v.Type, v.Str})
	case ValueTypeNumber:
		return json.Marshal(struct {
			Type  ValueType `json:"type"`
			Value float64   `json:"value"`
		}{v.Type, v.Num})
	case ValueTypeBoolean:
		return json.Marshal(struct {
			Type  ValueType `json:"type"`
			Value bool      `json:"value"`
		}{v.Type, v.Bool})
	case ValueTypeNull:
		return json.Marshal(struct {
			Type  ValueType `json:"type"`
			Value any       `json:"value"`
		}{v.Type, nil})
	default:
		return nil, fmt.Errorf("apicontract: TypedValue: unknown type %q", v.Type)
	}
}

// UnmarshalJSON parses the wire shape strictly: only "type" and "value" are
// accepted fields, at most once each (duplicate keys are checked directly -
// not just left to a caller's DecodeStrict - since TypedValue.UnmarshalJSON
// is invoked by encoding/json's own machinery whenever this type is embedded
// in a larger structure, bypassing any outer DecodeStrict call), value's
// JSON kind must match what Type requires (no string/number coercion), and
// the parsed result must pass Validate() before it is accepted.
func (v *TypedValue) UnmarshalJSON(data []byte) error {
	if err := checkNoDuplicateKeys(data); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("apicontract: TypedValue: %w", err)
	}
	for k := range raw {
		if k != "type" && k != "value" {
			return fmt.Errorf("apicontract: TypedValue: unknown field %q", k)
		}
	}
	typeRaw, ok := raw["type"]
	if !ok {
		return fmt.Errorf("apicontract: TypedValue: missing \"type\"")
	}
	var t ValueType
	if err := json.Unmarshal(typeRaw, &t); err != nil {
		return fmt.Errorf("apicontract: TypedValue: \"type\" must be a string: %w", err)
	}
	valueRaw, hasValue := raw["value"]
	if !hasValue {
		return fmt.Errorf("apicontract: TypedValue: missing \"value\"")
	}

	var parsed TypedValue
	switch t {
	case ValueTypeString, ValueTypeInteger, ValueTypeDecimal, ValueTypeDate, ValueTypeDatetime:
		var s string
		if err := json.Unmarshal(valueRaw, &s); err != nil {
			return fmt.Errorf("apicontract: TypedValue(%s): \"value\" must be a JSON string: %w", t, err)
		}
		parsed = TypedValue{Type: t, Str: s}
	case ValueTypeNumber:
		var n float64
		if err := json.Unmarshal(valueRaw, &n); err != nil {
			return fmt.Errorf("apicontract: TypedValue(number): \"value\" must be a JSON number: %w", err)
		}
		parsed = TypedValue{Type: t, Num: n}
	case ValueTypeBoolean:
		var b bool
		if err := json.Unmarshal(valueRaw, &b); err != nil {
			return fmt.Errorf("apicontract: TypedValue(boolean): \"value\" must be a JSON boolean: %w", err)
		}
		parsed = TypedValue{Type: t, Bool: b}
	case ValueTypeNull:
		if string(bytes.TrimSpace(valueRaw)) != "null" {
			return fmt.Errorf("apicontract: TypedValue(null): \"value\" must be JSON null")
		}
		parsed = TypedValue{Type: t}
	default:
		return fmt.Errorf("apicontract: TypedValue: unknown type %q", t)
	}

	if err := parsed.Validate(); err != nil {
		return fmt.Errorf("apicontract: TypedValue: %w", err)
	}
	*v = parsed
	return nil
}

// canonicalIntegerPattern: no leading '+', no leading zeros except a bare
// "0", and "-0" is rejected (a redundant sign on zero is not canonical).
var canonicalIntegerPattern = regexp.MustCompile(`^(0|-?[1-9][0-9]*)$`)

// canonicalDecimalPattern: an integer part with the same no-leading-zero
// rule (but "-0.5" is legitimate - the sign belongs to the whole value, not
// to a standalone "-0"), an optional fractional part whose digits (including
// trailing zeros) are preserved verbatim - "preserve precision" per the
// appendix. No scientific notation: that is what the "number" type is for.
var canonicalDecimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)

// Validate enforces the appendix's per-type content rules: "finite, exactly
// representable JSON number" for number; "canonical decimal integer, no
// leading +/zeros" for integer; "canonical decimal; preserve precision" for
// decimal; "YYYY-MM-DD, valid calendar date" for date; "RFC3339 normalized
// to UTC" for datetime. string/boolean/null carry no further content
// constraint - false, zero and empty string are valid present values.
func (v TypedValue) Validate() error {
	switch v.Type {
	case ValueTypeString, ValueTypeBoolean, ValueTypeNull:
		return nil
	case ValueTypeNumber:
		if math.IsInf(v.Num, 0) || math.IsNaN(v.Num) {
			return &ValidationError{Field: "value", Message: "must be a finite number"}
		}
		return nil
	case ValueTypeInteger:
		if !canonicalIntegerPattern.MatchString(v.Str) {
			return &ValidationError{Field: "value", Message: fmt.Sprintf("must be a canonical decimal integer (no leading +/zeros), got %q", v.Str)}
		}
		return nil
	case ValueTypeDecimal:
		if !canonicalDecimalPattern.MatchString(v.Str) {
			return &ValidationError{Field: "value", Message: fmt.Sprintf("must be a canonical decimal, got %q", v.Str)}
		}
		return nil
	case ValueTypeDate:
		t, err := time.Parse("2006-01-02", v.Str)
		if err != nil || t.Format("2006-01-02") != v.Str {
			return &ValidationError{Field: "value", Message: fmt.Sprintf("must be a valid YYYY-MM-DD calendar date, got %q", v.Str)}
		}
		return nil
	case ValueTypeDatetime:
		if _, err := time.Parse(time.RFC3339, v.Str); err != nil {
			return &ValidationError{Field: "value", Message: fmt.Sprintf("must be RFC3339, got %q: %v", v.Str, err)}
		}
		if !strings.HasSuffix(v.Str, "Z") {
			return &ValidationError{Field: "value", Message: fmt.Sprintf("must be normalized to UTC (\"Z\" suffix), got %q", v.Str)}
		}
		return nil
	default:
		return &ValidationError{Field: "type", Message: fmt.Sprintf("unknown TypedValue type %q", v.Type)}
	}
}
