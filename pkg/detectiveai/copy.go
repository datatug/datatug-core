package detectiveai

import (
	"slices"

	"github.com/datatug/datatug-core/pkg/detective"
)

func cloneSnapshot(value Snapshot) Snapshot {
	value.Environments = slices.Clone(value.Environments)
	cloneList := func(items []Resource) []Resource {
		copy := slices.Clone(items)
		for i := range copy {
			copy[i].Inputs = slices.Clone(copy[i].Inputs)
		}
		return copy
	}
	value.Queries = cloneList(value.Queries)
	value.Checks = cloneList(value.Checks)
	value.Validators = cloneList(value.Validators)
	value.Scenarios = cloneList(value.Scenarios)
	value.Cases = cloneList(value.Cases)
	value.Runs = cloneList(value.Runs)
	return value
}

func cloneInput(value detective.InputValue) detective.InputValue {
	if value.Scalar != nil {
		scalar := *value.Scalar
		value.Scalar = &scalar
	}
	if value.Set != nil {
		set := *value.Set
		set.Values = append(set.Values[:0:0], set.Values...)
		value.Set = &set
	}
	if value.Range != nil {
		r := *value.Range
		value.Range = &r
	}
	return value
}

func cloneInputs(values map[string]detective.InputValue) map[string]detective.InputValue {
	if values == nil {
		return nil
	}
	copy := make(map[string]detective.InputValue, len(values))
	for key, value := range values {
		copy[key] = cloneInput(value)
	}
	return copy
}

func cloneValidator(value detective.Validator) detective.Validator {
	value.Bindings = append([]detective.Binding(nil), value.Bindings...)
	if value.Expected != nil {
		expected := *value.Expected
		value.Expected = &expected
	}
	return value
}

func cloneDraft(value Draft) Draft {
	if value.Context != nil {
		context := make(map[string]ExtractedValue, len(value.Context))
		for key, item := range value.Context {
			item.Value = cloneInput(item.Value)
			item.Provenance = append([]Provenance(nil), item.Provenance...)
			context[key] = item
		}
		value.Context = context
	}
	if value.Query != nil {
		query := *value.Query
		query.Inputs = append([]detective.Input(nil), query.Inputs...)
		value.Query = &query
	}
	if value.Check != nil {
		check := *value.Check
		check.Inputs = append([]detective.Input(nil), check.Inputs...)
		check.Steps = append([]detective.Step(nil), check.Steps...)
		for i := range check.Steps {
			step := &check.Steps[i]
			step.Bindings = append([]detective.Binding(nil), step.Bindings...)
			step.Validators = append([]detective.Validator(nil), step.Validators...)
			for j := range step.Validators {
				step.Validators[j] = cloneValidator(step.Validators[j])
			}
		}
		value.Check = &check
	}
	if value.Validator != nil {
		validator := *value.Validator
		validator.Validator = cloneValidator(validator.Validator)
		value.Validator = &validator
	}
	if value.Scenario != nil {
		scenario := *value.Scenario
		scenario.Inputs = append([]detective.Input(nil), scenario.Inputs...)
		scenario.Steps = append([]detective.ScenarioStep(nil), scenario.Steps...)
		value.Scenario = &scenario
	}
	if value.Case != nil {
		caseEdit := *value.Case
		caseEdit.Context = cloneInputs(caseEdit.Context)
		caseEdit.Findings = append([]detective.Finding(nil), caseEdit.Findings...)
		for i := range caseEdit.Findings {
			caseEdit.Findings[i].EvidenceRefs = append([]string(nil), caseEdit.Findings[i].EvidenceRefs...)
		}
		value.Case = &caseEdit
	}
	if value.Run != nil {
		run := *value.Run
		run.Inputs = cloneInputs(run.Inputs)
		value.Run = &run
	}
	if value.Explanation != nil {
		explanation := *value.Explanation
		value.Explanation = &explanation
	}
	value.Sources = append([]Provenance(nil), value.Sources...)
	value.Ambiguities = append([]string(nil), value.Ambiguities...)
	return value
}

func cloneProposal(value Proposal) Proposal {
	value.Basis = cloneSnapshot(value.Basis)
	value.Draft = cloneDraft(value.Draft)
	value.Sources = append([]SourceEvidence(nil), value.Sources...)
	value.ExpectedCounts = cloneCounts(value.ExpectedCounts)
	return value
}

func cloneCounts(value map[string]int) map[string]int {
	if value == nil {
		return nil
	}
	copy := make(map[string]int, len(value))
	for key, count := range value {
		copy[key] = count
	}
	return copy
}
