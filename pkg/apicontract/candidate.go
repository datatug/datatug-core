package apicontract

import "fmt"

const (
	CandidateStateRunnable          = "runnable"
	CandidateStateNeedsInput        = "needs-input"
	CandidateStateNeedsTarget       = "needs-target"
	CandidateStateSourceUnavailable = "source-unavailable"
)

// CandidateTarget is one authorized eligible source a saved query could run
// against.
type CandidateTarget struct {
	Source string `json:"source"`
	Label  string `json:"label"`
}

func (t CandidateTarget) Validate() error {
	if err := requireNonEmpty("source", t.Source); err != nil {
		return err
	}
	return requireNonEmpty("label", t.Label)
}

// ChainStep explains, per parameter, how its value was (or would be)
// derived - declared, inferred or manual - distinctly. "a missing parameter
// still has an explanation."
type ChainStep struct {
	ParameterID  string     `json:"parameterId"`
	FactID       string     `json:"factId,omitempty"`
	ValueFactIDs [][]string `json:"valueFactIds,omitempty"`
	Explanation  string     `json:"explanation"`
}

func (c ChainStep) Validate() error {
	if err := requireCanonicalString("parameterId", c.ParameterID); err != nil {
		return err
	}
	if err := requireNonEmpty("explanation", c.Explanation); err != nil {
		return err
	}
	if c.FactID != "" && c.ValueFactIDs != nil {
		return &ValidationError{Field: "factId/valueFactIds", Message: "must not both be present"}
	}
	if c.FactID != "" {
		return validateFactID("factId", c.FactID)
	}
	if c.ValueFactIDs != nil {
		return validateFactIDGroups(c.ValueFactIDs)
	}
	return nil
}

// Ambiguous names a parameter with more than one distinct candidate value
// within the highest eligible automatic binding tier.
type Ambiguous struct {
	ParameterID string   `json:"parameterId"`
	FactIDs     []string `json:"factIds"`
}

func (a Ambiguous) Validate() error {
	if err := requireNonEmpty("parameterId", a.ParameterID); err != nil {
		return err
	}
	if len(a.FactIDs) == 0 {
		return &ValidationError{Field: "factIds", Message: "an ambiguous parameter must name at least one candidate fact"}
	}
	return nil
}

// Candidate is one query's runnability against the caller's current facts:
// its authorized eligible targets, the bindings/chain it would apply, what
// is still missing or ambiguous, and its overall State.
// api-contract.md "Endpoint table".
type Candidate struct {
	QueryID string            `json:"queryId"`
	Targets []CandidateTarget `json:"targets"`
	// SelectedSource is present only when exactly one eligible target
	// remains - "present only when one eligible target remains".
	SelectedSource string      `json:"selectedSource,omitempty"`
	Bindings       []Binding   `json:"bindings"`
	Chain          []ChainStep `json:"chain"`
	Missing        []string    `json:"missing"`
	Ambiguous      []Ambiguous `json:"ambiguous"`
	State          string      `json:"state"`
}

// Validate enforces QueryID is required, every Target/Binding/ChainStep/
// Ambiguous entry is itself valid, SelectedSource is set only when there is
// exactly one Target, and State is one of the closed set.
func (c Candidate) Validate() error {
	if err := requireNonEmpty("queryId", c.QueryID); err != nil {
		return err
	}
	for i, t := range c.Targets {
		if err := t.Validate(); err != nil {
			return &ValidationError{Field: "targets", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	if c.SelectedSource != "" && len(c.Targets) != 1 {
		return &ValidationError{Field: "selectedSource", Message: fmt.Sprintf("must be set only when exactly one target remains, got %d", len(c.Targets))}
	}
	if err := validateBindings("bindings", c.Bindings); err != nil {
		return err
	}
	bindingsByParameter := make(map[string]Binding, len(c.Bindings))
	for _, binding := range c.Bindings {
		bindingsByParameter[binding.ParameterID] = binding
	}
	chainByParameter := make(map[string]ChainStep, len(c.Chain))
	for i, step := range c.Chain {
		if err := step.Validate(); err != nil {
			return &ValidationError{Field: "chain", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		if _, exists := chainByParameter[step.ParameterID]; exists {
			return &ValidationError{Field: "chain", Message: fmt.Sprintf("duplicate parameter %q", step.ParameterID)}
		}
		chainByParameter[step.ParameterID] = step
		if binding, bound := bindingsByParameter[step.ParameterID]; bound {
			if binding.FactID != step.FactID || !equalFactIDGroups(binding.ValueFactIDs, step.ValueFactIDs) {
				return &ValidationError{Field: "chain", Message: fmt.Sprintf("parameter %q provenance must match its binding", step.ParameterID)}
			}
		} else if step.FactID != "" || step.ValueFactIDs != nil {
			return &ValidationError{Field: "chain", Message: fmt.Sprintf("parameter %q attributes facts without a binding", step.ParameterID)}
		}
	}
	for _, binding := range c.Bindings {
		if _, exists := chainByParameter[binding.ParameterID]; !exists {
			return &ValidationError{Field: "chain", Message: fmt.Sprintf("missing step for bound parameter %q", binding.ParameterID)}
		}
	}
	for i, a := range c.Ambiguous {
		if err := a.Validate(); err != nil {
			return &ValidationError{Field: "ambiguous", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	if err := requireOneOf("state", c.State, CandidateStateRunnable, CandidateStateNeedsInput, CandidateStateNeedsTarget, CandidateStateSourceUnavailable); err != nil {
		return err
	}
	return nil
}

func equalFactIDGroups(left, right [][]string) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for i := range left {
		if len(left[i]) != len(right[i]) {
			return false
		}
		for j := range left[i] {
			if left[i][j] != right[i][j] {
				return false
			}
		}
	}
	return true
}
