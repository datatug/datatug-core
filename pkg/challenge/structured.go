package challenge

import (
	"fmt"
	"sort"
)

// StructuredPart is a closed selection answer: every field and option is
// declared, and grading is exact value/set equality. No free text, model or
// fuzzy matching is involved.
type StructuredPart struct {
	Fields   []StructuredField `json:"fields"`
	Expected StructuredAnswer  `json:"expected"`
}

// StructuredField declares one closed field. Single fields always require
// exactly one option; multi fields declare an inclusive min/max selection
// count.
type StructuredField struct {
	ID            string             `json:"id"`
	Label         string             `json:"label"`
	Cardinality   string             `json:"cardinality"`
	Required      bool               `json:"required"`
	MinSelections int                `json:"minSelections,omitempty"`
	MaxSelections int                `json:"maxSelections,omitempty"`
	Options       []StructuredOption `json:"options"`
}

// StructuredOption is one closed option ID and its learner-facing label.
type StructuredOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// StructuredAnswer is both the expected answer and a learner submission.
type StructuredAnswer struct {
	Selections map[string][]string `json:"selections"`
}

// Stable structured-grading problem codes.
const (
	ProblemUnknownField      = "unknown-field"
	ProblemUnknownOption     = "unknown-option"
	ProblemDuplicateOption   = "duplicate-option"
	ProblemMissingSelection  = "missing-selection"
	ProblemSingleCardinality = "single-cardinality"
	ProblemSelectionCount    = "selection-cardinality"
	ProblemSelectionMismatch = "selection-mismatch"
)

// FieldOutcome reports one field's deterministic result.
type FieldOutcome struct {
	FieldID string `json:"fieldId"`
	Outcome string `json:"outcome"`
	Problem string `json:"problem,omitempty"`
	Message string `json:"message,omitempty"`
}

// StructuredResult is the outcome of a closed-selection answer.
type StructuredResult struct {
	Outcome         string         `json:"outcome"`
	ExecutionStatus string         `json:"executionStatus"`
	Problem         string         `json:"problem,omitempty"`
	Message         string         `json:"message,omitempty"`
	Fields          []FieldOutcome `json:"fields"`
}

// AsOutcome projects the result onto the attempt contract.
func (r StructuredResult) AsOutcome(part Part) PartOutcome {
	return PartOutcome{
		PartID:          part.ID,
		AnswerKind:      part.AnswerKind,
		Required:        part.Required,
		Outcome:         r.Outcome,
		ExecutionStatus: r.ExecutionStatus,
		Problem:         r.Problem,
		Message:         r.Message,
	}
}

// Validate checks the closed field/option space and the expected answer.
func (s StructuredPart) Validate() error {
	if len(s.Fields) == 0 || len(s.Fields) > maxStructuredField {
		return fmt.Errorf("structured parts require 1..%d fields", maxStructuredField)
	}
	fields := map[string]StructuredField{}
	for _, field := range s.Fields {
		if !validID(field.ID) || !boundedText(field.Label, 1, 200) || fields[field.ID].ID != "" {
			return fmt.Errorf("structured fields require valid unique ids and 1..200 character labels")
		}
		fields[field.ID] = field
		if !oneOf(field.Cardinality, CardinalitySingle, CardinalityMulti) {
			return fmt.Errorf("field %q cardinality must be single or multi", field.ID)
		}
		if len(field.Options) == 0 || len(field.Options) > maxStructuredOpts {
			return fmt.Errorf("field %q requires 1..%d options", field.ID, maxStructuredOpts)
		}
		options := map[string]bool{}
		for _, option := range field.Options {
			if !validID(option.ID) || !boundedText(option.Label, 1, 200) || options[option.ID] {
				return fmt.Errorf("field %q options require valid unique ids and 1..200 character labels", field.ID)
			}
			options[option.ID] = true
		}
		switch field.Cardinality {
		case CardinalitySingle:
			if field.MinSelections != 0 || field.MaxSelections != 0 {
				return fmt.Errorf("single field %q must not declare minSelections/maxSelections", field.ID)
			}
		case CardinalityMulti:
			if field.MinSelections < 0 || field.MaxSelections < 1 || field.MinSelections > field.MaxSelections || field.MaxSelections > len(field.Options) {
				return fmt.Errorf("multi field %q requires 0 <= minSelections <= maxSelections <= options", field.ID)
			}
		}
	}

	for fieldID, selections := range s.Expected.Selections {
		field, ok := fields[fieldID]
		if !ok {
			return fmt.Errorf("expected answer names unknown field %q", fieldID)
		}
		if err := validateSelection(field, selections); err != nil {
			return fmt.Errorf("expected answer field %q: %w", fieldID, err)
		}
	}
	for _, field := range s.Fields {
		if field.Required && len(s.Expected.Selections[field.ID]) == 0 {
			return fmt.Errorf("required field %q has no expected selection", field.ID)
		}
	}
	return nil
}

func validateSelection(field StructuredField, selections []string) error {
	options := map[string]bool{}
	for _, option := range field.Options {
		options[option.ID] = true
	}
	seen := map[string]bool{}
	for _, selection := range selections {
		if !options[selection] {
			return fmt.Errorf("unknown option %q", selection)
		}
		if seen[selection] {
			return fmt.Errorf("duplicate option %q", selection)
		}
		seen[selection] = true
	}
	switch field.Cardinality {
	case CardinalitySingle:
		if len(selections) != 1 {
			return fmt.Errorf("single field requires exactly one option")
		}
	case CardinalityMulti:
		if len(selections) < field.MinSelections || len(selections) > field.MaxSelections {
			return fmt.Errorf("multi field requires %d..%d options", field.MinSelections, field.MaxSelections)
		}
	}
	return nil
}

// EvaluateStructured grades a closed selection without AI judgement.
func EvaluateStructured(part StructuredPart, answer StructuredAnswer) StructuredResult {
	fields := map[string]StructuredField{}
	for _, field := range part.Fields {
		fields[field.ID] = field
	}
	for fieldID := range answer.Selections {
		if _, ok := fields[fieldID]; !ok {
			return StructuredResult{
				Outcome:         OutcomeUnknown,
				ExecutionStatus: StatusError,
				Problem:         ProblemUnknownField,
				Message:         fmt.Sprintf("unknown structured field %q", fieldID),
			}
		}
	}

	result := StructuredResult{Outcome: OutcomePass, ExecutionStatus: StatusCompleted}
	for _, field := range part.Fields {
		selections, present := answer.Selections[field.ID]
		expected := part.Expected.Selections[field.ID]
		if !present || len(selections) == 0 {
			if !field.Required && len(expected) == 0 {
				result.Fields = append(result.Fields, FieldOutcome{FieldID: field.ID, Outcome: OutcomePass})
				continue
			}
			outcome := FieldOutcome{
				FieldID: field.ID,
				Outcome: OutcomeFail,
				Problem: ProblemMissingSelection,
				Message: fmt.Sprintf("field %q requires a selection", field.ID),
			}
			result.Fields = append(result.Fields, outcome)
			result.fail(outcome)
			continue
		}
		if err := validateSelection(field, selections); err != nil {
			problem := ProblemSelectionCount
			if containsUnknownOption(field, selections) {
				problem = ProblemUnknownOption
			}
			if hasDuplicate(selections) {
				problem = ProblemDuplicateOption
			}
			if field.Cardinality == CardinalitySingle && len(selections) != 1 {
				problem = ProblemSingleCardinality
			}
			outcome := FieldOutcome{FieldID: field.ID, Outcome: OutcomeUnknown, Problem: problem, Message: err.Error()}
			result.Fields = append(result.Fields, outcome)
			result.ExecutionStatus = StatusError
			result.Outcome = OutcomeUnknown
			result.Problem = problem
			result.Message = fmt.Sprintf("field %q: %s", field.ID, err.Error())
			return result
		}
		if !sameSet(selections, expected) {
			outcome := FieldOutcome{
				FieldID: field.ID,
				Outcome: OutcomeFail,
				Problem: ProblemSelectionMismatch,
				Message: fmt.Sprintf("field %q expected %v, selected %v", field.ID, sortedCopy(expected), sortedCopy(selections)),
			}
			result.Fields = append(result.Fields, outcome)
			result.fail(outcome)
			continue
		}
		result.Fields = append(result.Fields, FieldOutcome{FieldID: field.ID, Outcome: OutcomePass})
	}
	return result
}

func (r *StructuredResult) fail(outcome FieldOutcome) {
	if r.Outcome == OutcomePass {
		r.Outcome = OutcomeFail
	}
	if r.Problem == "" {
		r.Problem = outcome.Problem
		r.Message = outcome.Message
	}
}

func containsUnknownOption(field StructuredField, selections []string) bool {
	known := map[string]bool{}
	for _, option := range field.Options {
		known[option.ID] = true
	}
	for _, selection := range selections {
		if !known[selection] {
			return true
		}
	}
	return false
}

func hasDuplicate(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if seen[value] {
			return true
		}
		seen[value] = true
	}
	return false
}

func sameSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := map[string]int{}
	for _, value := range left {
		seen[value]++
	}
	for _, value := range right {
		seen[value]--
		if seen[value] < 0 {
			return false
		}
	}
	return true
}

func sortedCopy(values []string) []string {
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return copied
}
