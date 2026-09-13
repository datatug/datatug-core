package apicontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

const (
	ValueTypeSet        = "set"
	maxTypedValueSetLen = 500
)

// TypedValueSet is the bounded, canonical cohort value transported for a
// parameter that explicitly supports set binding. Values are non-null scalar
// TypedValues of one type, sorted by their canonical JSON bytes and unique.
type TypedValueSet struct {
	Values []TypedValue `json:"values"`
}

// NewTypedValueSet deduplicates and orders values into the canonical wire
// representation required for deterministic execution and recording.
func NewTypedValueSet(values ...TypedValue) (TypedValueSet, error) {
	if len(values) == 0 || len(values) > maxTypedValueSetLen {
		return TypedValueSet{}, &ValidationError{Field: "values", Message: "must contain between 1 and 500 values"}
	}
	type encodedValue struct {
		value   TypedValue
		encoded []byte
	}
	encoded := make([]encodedValue, 0, len(values))
	for i, value := range values {
		if err := validateSetScalar(value); err != nil {
			return TypedValueSet{}, &ValidationError{Field: "values", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		data, err := json.Marshal(value)
		if err != nil {
			return TypedValueSet{}, fmt.Errorf("apicontract: canonicalize TypedValueSet: %w", err)
		}
		encoded = append(encoded, encodedValue{value: value, encoded: data})
	}
	sort.Slice(encoded, func(i, j int) bool { return bytes.Compare(encoded[i].encoded, encoded[j].encoded) < 0 })
	result := TypedValueSet{Values: make([]TypedValue, 0, len(encoded))}
	for i, item := range encoded {
		if i > 0 && bytes.Equal(item.encoded, encoded[i-1].encoded) {
			continue
		}
		result.Values = append(result.Values, item.value)
	}
	if err := result.Validate(); err != nil {
		return TypedValueSet{}, err
	}
	return result, nil
}

func (s TypedValueSet) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type   string       `json:"type"`
		Values []TypedValue `json:"values"`
	}{Type: ValueTypeSet, Values: s.Values})
}

func (s *TypedValueSet) UnmarshalJSON(data []byte) error {
	if err := checkNoDuplicateKeys(data); err != nil {
		return err
	}
	var raw struct {
		Type   string       `json:"type"`
		Values []TypedValue `json:"values"`
	}
	if err := DecodeStrict(data, &raw); err != nil {
		return fmt.Errorf("apicontract: TypedValueSet: %w", err)
	}
	if raw.Type != ValueTypeSet {
		return fmt.Errorf("apicontract: TypedValueSet: type must be %q", ValueTypeSet)
	}
	parsed := TypedValueSet{Values: raw.Values}
	if err := parsed.Validate(); err != nil {
		return fmt.Errorf("apicontract: TypedValueSet: %w", err)
	}
	*s = parsed
	return nil
}

func (s TypedValueSet) Validate() error {
	if len(s.Values) == 0 || len(s.Values) > maxTypedValueSetLen {
		return &ValidationError{Field: "values", Message: "must contain between 1 and 500 values"}
	}
	var previous []byte
	var valueType ValueType
	for i, value := range s.Values {
		if err := validateSetScalar(value); err != nil {
			return &ValidationError{Field: "values", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		if i == 0 {
			valueType = value.Type
		} else if value.Type != valueType {
			return &ValidationError{Field: "values", Message: "all values must have one scalar type"}
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return &ValidationError{Field: "values", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		if i > 0 && bytes.Compare(previous, encoded) >= 0 {
			return &ValidationError{Field: "values", Message: "must be unique and sorted by canonical JSON bytes"}
		}
		previous = encoded
	}
	return nil
}

func validateSetScalar(value TypedValue) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Type == ValueTypeNull {
		return &ValidationError{Field: "value", Message: "set values must not be null"}
	}
	return nil
}

// TypedValueOrSet is the exact wire union accepted in parameter and binding
// value positions. Its JSON is the contained scalar or set object, never an
// additional wrapper.
type TypedValueOrSet struct {
	Scalar *TypedValue
	Set    *TypedValueSet
}

func ScalarValue(value TypedValue) TypedValueOrSet { return TypedValueOrSet{Scalar: &value} }
func SetValue(value TypedValueSet) TypedValueOrSet { return TypedValueOrSet{Set: &value} }

func (v TypedValueOrSet) IsSet() bool { return v.Set != nil && v.Scalar == nil }

func (v TypedValueOrSet) Validate() error {
	if (v.Scalar == nil) == (v.Set == nil) {
		return &ValidationError{Field: "value", Message: "exactly one of scalar or set is required"}
	}
	if v.Set != nil {
		return v.Set.Validate()
	}
	return v.Scalar.Validate()
}

func (v TypedValueOrSet) MarshalJSON() ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	if v.Set != nil {
		return json.Marshal(v.Set)
	}
	return json.Marshal(v.Scalar)
}

func (v *TypedValueOrSet) UnmarshalJSON(data []byte) error {
	if err := checkNoDuplicateKeys(data); err != nil {
		return err
	}
	var discriminant struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &discriminant); err != nil {
		return fmt.Errorf("apicontract: TypedValueOrSet: %w", err)
	}
	if discriminant.Type == ValueTypeSet {
		var set TypedValueSet
		if err := json.Unmarshal(data, &set); err != nil {
			return err
		}
		*v = SetValue(set)
		return nil
	}
	var scalar TypedValue
	if err := json.Unmarshal(data, &scalar); err != nil {
		return err
	}
	*v = ScalarValue(scalar)
	return nil
}
