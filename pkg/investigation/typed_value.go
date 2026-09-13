package investigation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

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

// TypedValue is the canonical transport tagged union. Its JSON implementation
// intentionally preserves apicontract's existing strict wire semantics.
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

func (v *TypedValue) UnmarshalJSON(data []byte) error {
	if err := checkTypedValueKeys(data); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("apicontract: TypedValue: %w", err)
	}
	for key := range raw {
		if key != "type" && key != "value" {
			return fmt.Errorf("apicontract: TypedValue: unknown field %q", key)
		}
	}
	typeRaw, ok := raw["type"]
	if !ok {
		return fmt.Errorf("apicontract: TypedValue: missing \"type\"")
	}
	var valueType ValueType
	if err := json.Unmarshal(typeRaw, &valueType); err != nil {
		return fmt.Errorf("apicontract: TypedValue: \"type\" must be a string: %w", err)
	}
	valueRaw, ok := raw["value"]
	if !ok {
		return fmt.Errorf("apicontract: TypedValue: missing \"value\"")
	}

	var parsed TypedValue
	switch valueType {
	case ValueTypeString, ValueTypeInteger, ValueTypeDecimal, ValueTypeDate, ValueTypeDatetime:
		var value string
		if err := json.Unmarshal(valueRaw, &value); err != nil {
			return fmt.Errorf("apicontract: TypedValue(%s): \"value\" must be a JSON string: %w", valueType, err)
		}
		parsed = TypedValue{Type: valueType, Str: value}
	case ValueTypeNumber:
		var value float64
		if err := json.Unmarshal(valueRaw, &value); err != nil {
			return fmt.Errorf("apicontract: TypedValue(number): \"value\" must be a JSON number: %w", err)
		}
		parsed = TypedValue{Type: valueType, Num: value}
	case ValueTypeBoolean:
		var value bool
		if err := json.Unmarshal(valueRaw, &value); err != nil {
			return fmt.Errorf("apicontract: TypedValue(boolean): \"value\" must be a JSON boolean: %w", err)
		}
		parsed = TypedValue{Type: valueType, Bool: value}
	case ValueTypeNull:
		if string(bytes.TrimSpace(valueRaw)) != "null" {
			return fmt.Errorf("apicontract: TypedValue(null): \"value\" must be JSON null")
		}
		parsed = TypedValue{Type: valueType}
	default:
		return fmt.Errorf("apicontract: TypedValue: unknown type %q", valueType)
	}
	if err := parsed.Validate(); err != nil {
		return fmt.Errorf("apicontract: TypedValue: %w", err)
	}
	*v = parsed
	return nil
}

// checkTypedValueKeys rejects duplicate object keys before encoding/json can
// collapse them. TypedValue permits scalar children only, so no recursive
// duplicate-key walk is necessary here.
func checkTypedValueKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("apicontract: %w", err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return nil
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return fmt.Errorf("apicontract: %w", err)
		}
		key, _ := token.(string)
		if seen[key] {
			return fmt.Errorf("apicontract: duplicate key %q", key)
		}
		seen[key] = true
		if err := decoder.Decode(new(json.RawMessage)); err != nil {
			return fmt.Errorf("apicontract: %w", err)
		}
	}
	return nil
}

var canonicalIntegerPattern = regexp.MustCompile(`^(0|-?[1-9][0-9]*)$`)
var canonicalDecimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)

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
		parsed, err := time.Parse("2006-01-02", v.Str)
		if err != nil || parsed.Format("2006-01-02") != v.Str {
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
