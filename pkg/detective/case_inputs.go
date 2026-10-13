package detective

import (
	"fmt"
	"reflect"

	"github.com/datatug/datatug-core/pkg/investigation"
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

// MatchScenarioQueryCohort checks a query step's server-recorded parameter
// values against the Case context. Range inputs are reconstructed only from
// their bound scalar endpoints; a caller cannot supply a separate cohort proof.
func MatchScenarioQueryCohort(step ScenarioStep, caseContext, recordedParams map[string]InputValue) error {
	if step.Kind != "query" || validateBindings(step.Bindings) != nil {
		return ErrInvalid
	}
	bound := make(map[string]InputValue, len(caseContext))
	rangeParts := make(map[string]*scenarioRangeParts)
	unmappedRangeParts := make(map[string]*scenarioRangeParts)
	for _, binding := range step.Bindings {
		if binding.FromColumn != "" {
			return ErrInvalid
		}
		parameter, ok := recordedParams[binding.Input]
		if !ok || parameter.Validate() != nil || parameter.Range != nil {
			return ErrInvalid
		}
		source, inCase := caseContext[binding.FromInput]
		if !inCase {
			if binding.Part != "" {
				if parameter.Scalar == nil {
					return ErrInvalid
				}
				if err := addScenarioRangePart(unmappedRangeParts, binding.FromInput, binding.Part, *parameter.Scalar); err != nil {
					return err
				}
			}
			continue
		}
		if source.Validate() != nil {
			return ErrInvalid
		}
		if source.Range != nil {
			if parameter.Scalar == nil {
				return ErrInvalid
			}
			endpoint := source.Range.Start
			if binding.Part == "end" {
				endpoint = source.Range.End
			} else if binding.Part != "start" {
				return ErrInvalid
			}
			if parameter.Scalar.Type != endpoint.Type {
				return ErrInvalid
			}
			if err := addScenarioRangePart(rangeParts, binding.FromInput, binding.Part, *parameter.Scalar); err != nil {
				return err
			}
			continue
		}
		if binding.Part != "" || (source.Scalar == nil) != (parameter.Scalar == nil) || (source.Set == nil) != (parameter.Set == nil) {
			return ErrInvalid
		}
		if prior, exists := bound[binding.FromInput]; exists && !reflect.DeepEqual(prior, parameter) {
			return ErrInvalid
		}
		bound[binding.FromInput] = parameter
	}
	for _, parts := range unmappedRangeParts {
		if !parts.hasStart || !parts.hasEnd || parts.start.Type != parts.end.Type || !validRangeInputType(parts.start.Type) {
			return ErrInvalid
		}
		if (InputValue{Range: &ValueRange{Type: "range", Start: parts.start, End: parts.end}}).Validate() != nil {
			return ErrInvalid
		}
	}
	for name, value := range caseContext {
		if value.Range != nil {
			parts := rangeParts[name]
			if parts == nil || !parts.hasStart || !parts.hasEnd {
				return ErrInvalid
			}
			bound[name] = InputValue{Range: &ValueRange{Type: value.Range.Type, Start: parts.start, End: parts.end}}
			continue
		}
		if _, exists := bound[name]; exists {
			continue
		}
		if direct, exists := recordedParams[name]; exists {
			if direct.Validate() != nil {
				return ErrInvalid
			}
			bound[name] = direct
		}
	}
	return MatchCaseCohort(caseContext, bound)
}

type scenarioRangeParts struct {
	start    Value
	end      Value
	hasStart bool
	hasEnd   bool
}

func addScenarioRangePart(partsByInput map[string]*scenarioRangeParts, input, part string, value Value) error {
	parts := partsByInput[input]
	if parts == nil {
		parts = &scenarioRangeParts{}
		partsByInput[input] = parts
	}
	switch part {
	case "start":
		if parts.hasStart {
			return ErrInvalid
		}
		parts.start, parts.hasStart = value, true
	case "end":
		if parts.hasEnd {
			return ErrInvalid
		}
		parts.end, parts.hasEnd = value, true
	default:
		return ErrInvalid
	}
	return nil
}

func validRangeInputType(value investigation.ValueType) bool {
	switch value {
	case investigation.ValueTypeDate, investigation.ValueTypeDatetime, investigation.ValueTypeInteger, investigation.ValueTypeDecimal, investigation.ValueTypeNumber:
		return true
	default:
		return false
	}
}
