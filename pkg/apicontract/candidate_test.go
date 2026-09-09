package apicontract

import (
	"encoding/json"
	"testing"
)

func validCandidate() Candidate {
	return Candidate{
		QueryID:  "customer-invoices",
		Targets:  []CandidateTarget{{Source: "chinook-local", Label: "Local Chinook"}},
		Bindings: []Binding{},
		Chain: []ChainStep{
			{ParameterID: "CustomerId", FactID: "f1", Explanation: "declared mapping Customer.ID"},
		},
		Missing:   []string{},
		Ambiguous: []Ambiguous{},
		State:     "runnable",
	}
}

func TestCandidate_JSONFieldNames(t *testing.T) {
	c := validCandidate()
	c.SelectedSource = "chinook-local"
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"queryId", "targets", "selectedSource", "bindings", "chain", "missing", "ambiguous", "state"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("missing key %q in %s", key, data)
		}
	}
}

func TestCandidate_SelectedSourceOmittedWhenAbsent(t *testing.T) {
	c := validCandidate()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	_ = json.Unmarshal(data, &generic)
	if _, ok := generic["selectedSource"]; ok {
		t.Errorf("selectedSource must be omitted when absent, got %s", data)
	}
}

func TestCandidate_Validate(t *testing.T) {
	if err := validCandidate().Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	missingQueryID := validCandidate()
	missingQueryID.QueryID = ""
	if err := missingQueryID.Validate(); err == nil {
		t.Error("expected an error: missing queryId")
	}

	badState := validCandidate()
	badState.State = "bogus"
	if err := badState.Validate(); err == nil {
		t.Error("expected an error: invalid state")
	}

	badTarget := validCandidate()
	badTarget.Targets = []CandidateTarget{{Label: "no source"}}
	if err := badTarget.Validate(); err == nil {
		t.Error("expected an error: target missing source")
	}

	badTargetNoLabel := validCandidate()
	badTargetNoLabel.Targets = []CandidateTarget{{Source: "chinook-local"}}
	if err := badTargetNoLabel.Validate(); err == nil {
		t.Error("expected an error: target missing label")
	}

	badBinding := validCandidate()
	badBinding.Bindings = []Binding{{ParameterID: "p", Value: NewIntegerValue("bad"), Origin: "selection", OriginEvidence: "client-reported"}}
	if err := badBinding.Validate(); err == nil {
		t.Error("expected an error: invalid binding")
	}
}

func TestCandidate_Validate_SelectedSourceOnlyWithExactlyOneTarget(t *testing.T) {
	c := validCandidate()
	c.SelectedSource = "chinook-local"
	if err := c.Validate(); err != nil {
		t.Errorf("expected valid with exactly one target, got: %v", err)
	}

	c.Targets = append(c.Targets, CandidateTarget{Source: "chinook-prod", Label: "Prod Chinook"})
	if err := c.Validate(); err == nil {
		t.Error("expected an error: selectedSource set with more than one target")
	}

	c.Targets = nil
	if err := c.Validate(); err == nil {
		t.Error("expected an error: selectedSource set with zero targets")
	}
}

func TestCandidate_Validate_Chain(t *testing.T) {
	c := validCandidate()
	c.Chain = []ChainStep{{Explanation: "no parameterId"}}
	if err := c.Validate(); err == nil {
		t.Error("expected an error: chain step missing parameterId")
	}

	c2 := validCandidate()
	c2.Chain = []ChainStep{{ParameterID: "X"}}
	if err := c2.Validate(); err == nil {
		t.Error("expected an error: chain step missing explanation")
	}
}

func TestCandidate_Validate_MissingParameterStillHasChainExplanation(t *testing.T) {
	// "a missing parameter still has an explanation"
	c := validCandidate()
	c.State = "needs-input"
	c.Missing = []string{"CustomerId"}
	c.Chain = []ChainStep{{ParameterID: "CustomerId", Explanation: "no selected or context fact for Customer.ID"}}
	if err := c.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestAmbiguous_Validate_Valid(t *testing.T) {
	if err := (Ambiguous{ParameterID: "CustomerId", FactIDs: []string{"f1", "f2"}}).Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestCandidate_Validate_Ambiguous(t *testing.T) {
	c := validCandidate()
	c.State = "needs-input"
	c.Ambiguous = []Ambiguous{{FactIDs: []string{"f1", "f2"}}}
	if err := c.Validate(); err == nil {
		t.Error("expected an error: ambiguous entry missing parameterId")
	}

	c2 := validCandidate()
	c2.State = "needs-input"
	c2.Ambiguous = []Ambiguous{{ParameterID: "CustomerId", FactIDs: []string{}}}
	if err := c2.Validate(); err == nil {
		t.Error("expected an error: ambiguous entry with no candidate facts")
	}
}
