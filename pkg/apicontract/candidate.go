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
	ParameterID string `json:"parameterId"`
	FactID      string `json:"factId,omitempty"`
	Explanation string `json:"explanation"`
}

func (c ChainStep) Validate() error {
	if err := requireNonEmpty("parameterId", c.ParameterID); err != nil {
		return err
	}
	return requireNonEmpty("explanation", c.Explanation)
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
	for i, b := range c.Bindings {
		if err := b.Validate(); err != nil {
			return &ValidationError{Field: "bindings", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	for i, step := range c.Chain {
		if err := step.Validate(); err != nil {
			return &ValidationError{Field: "chain", Message: fmt.Sprintf("index %d: %s", i, err)}
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
