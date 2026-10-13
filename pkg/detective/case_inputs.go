package detective

import (
	"fmt"
	"reflect"
)

// BindCaseInputs merges the authoritative Case context into a Check's declared
// inputs. A declared Case value cannot be replaced by a caller-supplied value;
// a set or range in the Case must be declared by the Check. Unrelated scalar
// facts remain Case context and are not passed to the Check. Returned values
// own their nested storage so later caller edits cannot change accepted inputs.
func BindCaseInputs(schema []Input, supplied, context map[string]InputValue) (map[string]InputValue, error) {
	declared := make(map[string]bool, len(schema))
	for _, input := range schema {
		if !validParameterName(input.Name) || declared[input.Name] || !validInputType(input.Type) || !oneOf(input.Cardinality, "scalar", "set", "range") {
			return nil, fmt.Errorf("%w: invalid Check input schema", ErrInvalid)
		}
		declared[input.Name] = true
	}
	effective := make(map[string]InputValue, len(supplied)+len(context))
	for name, value := range supplied {
		effective[name] = value
	}
	for name, value := range context {
		if err := value.Validate(); err != nil {
			return nil, fmt.Errorf("%w: invalid Case context %s: %v", ErrInvalid, name, err)
		}
		if !declared[name] {
			if value.Set != nil || value.Range != nil {
				return nil, fmt.Errorf("%w: Check does not declare Case cohort %s", ErrInvalid, name)
			}
			continue
		}
		if given, exists := effective[name]; exists && !reflect.DeepEqual(given, value) {
			return nil, fmt.Errorf("%w: input %s conflicts with Case context", ErrInvalid, name)
		}
		effective[name] = value
	}
	if err := ValidateInputs(schema, effective); err != nil {
		return nil, err
	}
	for name, value := range effective {
		if value.Scalar != nil {
			copy := *value.Scalar
			value.Scalar = &copy
		}
		if value.Set != nil {
			copy := *value.Set
			copy.Values = append([]Value(nil), copy.Values...)
			value.Set = &copy
		}
		if value.Range != nil {
			copy := *value.Range
			value.Range = &copy
		}
		effective[name] = value
	}
	return effective, nil
}

// MatchCaseCohort verifies inputs from an ordinary query or retained execution
// against the Case's confirmed context. Set and range cohorts must be present
// exactly; unrelated scalar facts may be omitted, but cannot be contradicted.
func MatchCaseCohort(context, inputs map[string]InputValue) error {
	for name, value := range context {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("%w: invalid Case context %s: %v", ErrInvalid, name, err)
		}
		bound, exists := inputs[name]
		if !exists {
			if value.Set != nil || value.Range != nil {
				return fmt.Errorf("%w: missing Case cohort %s", ErrInvalid, name)
			}
			continue
		}
		if err := bound.Validate(); err != nil || !reflect.DeepEqual(bound, value) {
			return fmt.Errorf("%w: input %s conflicts with Case context", ErrInvalid, name)
		}
	}
	return nil
}
