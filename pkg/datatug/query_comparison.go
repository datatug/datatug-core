package datatug

import "fmt"

// QueryComparison declares an explicit pair and result roles. It grants no
// source authority: the provider must independently verify actual observations.
type QueryComparison struct {
	Kind         string                    `json:"kind" yaml:"kind"`
	PrimaryInput string                    `json:"primaryInput" yaml:"primaryInput"`
	ControlInput string                    `json:"controlInput" yaml:"controlInput"`
	Projection   QueryComparisonProjection `json:"projection" yaml:"projection"`
}

// QueryComparisonProjection maps one declared recordset; names may be aliases.
// It is not QuerySemanticModel provenance or a client-supplied physical mapping.
type QueryComparisonProjection struct {
	MemberColumn      string `json:"memberColumn" yaml:"memberColumn"`
	ObservationColumn string `json:"observationColumn" yaml:"observationColumn"`
	LaneColumn        string `json:"laneColumn" yaml:"laneColumn"`
	EventColumn       string `json:"eventColumn" yaml:"eventColumn"`
	SequenceColumn    string `json:"sequenceColumn" yaml:"sequenceColumn"`
	TimeColumn        string `json:"timeColumn" yaml:"timeColumn"`
	ClockDomainColumn string `json:"clockDomainColumn" yaml:"clockDomainColumn"`
	PrecisionColumn   string `json:"precisionColumn" yaml:"precisionColumn"`
	StatusColumn      string `json:"statusColumn" yaml:"statusColumn"`
	AnchorColumn      string `json:"anchorColumn" yaml:"anchorColumn"`
}

func (p QueryComparisonProjection) roles() []struct{ name, kind string } {
	return []struct{ name, kind string }{
		{p.MemberColumn, "integer"}, {p.ObservationColumn, "string"}, {p.LaneColumn, "string"}, {p.EventColumn, "string"},
		{p.SequenceColumn, "integer"}, {p.TimeColumn, "string"}, {p.ClockDomainColumn, "string"},
		{p.PrecisionColumn, "integer"}, {p.StatusColumn, "string"}, {p.AnchorColumn, "integer"},
	}
}

// Validate checks metadata shape and credential safety, never business IDs,
// timestamp arithmetic, event kinds or asserted source/anchor truth.
func (v QueryComparison) Validate() error {
	if v.Kind != "pair" {
		return fmt.Errorf("comparison kind must be pair")
	}
	if v.PrimaryInput == "" || v.ControlInput == "" || v.PrimaryInput == v.ControlInput {
		return fmt.Errorf("comparison requires distinct input IDs")
	}
	for _, id := range []string{v.PrimaryInput, v.ControlInput} {
		if reason, bad := identifierShapeReason(id); bad {
			return fmt.Errorf("comparison input: %s", reason)
		}
	}
	seen := map[string]bool{}
	for _, role := range v.Projection.roles() {
		if role.name == "" || seen[role.name] {
			return fmt.Errorf("comparison roles must be nonempty and distinct")
		}
		if reason, bad := identifierEnvelopeReason(role.name); bad {
			return fmt.Errorf("comparison column: %s", reason)
		}
		if reason, bad := EmbeddedCredentialReason(role.name); bad {
			return fmt.Errorf("comparison column: %s; %s", reason, storedQueryHint)
		}
		seen[role.name] = true
	}
	return nil
}

func (v QueryComparison) validateAgainst(parameters Parameters, recordsets []RecordsetDefinition) error {
	if err := v.Validate(); err != nil {
		return err
	}
	seen := map[string]bool{}
	found := 0
	for _, p := range parameters {
		if seen[p.ID] {
			return fmt.Errorf("comparison query has duplicate parameter IDs")
		}
		seen[p.ID] = true
		if p.ID == v.PrimaryInput || p.ID == v.ControlInput {
			if p.Type != "integer" || !p.IsRequired || p.IsMultiValue {
				return fmt.Errorf("comparison inputs must be required scalar integers")
			}
			found++
		}
	}
	if found != 2 {
		return fmt.Errorf("comparison inputs must be declared")
	}
	if len(recordsets) != 1 {
		return fmt.Errorf("comparison requires one declared recordset")
	}
	columns := map[string]string{}
	for _, c := range recordsets[0].Columns {
		if _, exists := columns[c.Name]; exists {
			return fmt.Errorf("comparison query has duplicate result columns")
		}
		columns[c.Name] = c.Type
	}
	for _, role := range v.Projection.roles() {
		if columns[role.name] != role.kind {
			return fmt.Errorf("comparison column %s must be declared as %s", role.name, role.kind)
		}
	}
	return nil
}
