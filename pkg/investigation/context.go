package investigation

import (
	"fmt"
	"strings"
)

type PhysicalRef struct {
	Source     string `json:"source"`
	Collection string `json:"collection"`
	Column     string `json:"column"`
}

func (r PhysicalRef) Validate() error {
	if err := requireNonEmpty("source", r.Source); err != nil {
		return err
	}
	if err := requireNonEmpty("collection", r.Collection); err != nil {
		return err
	}
	return requireNonEmpty("column", r.Column)
}

type Fact struct {
	ID       string       `json:"id"`
	Entity   string       `json:"entity"`
	Field    string       `json:"field"`
	Value    TypedValue   `json:"value"`
	Origin   string       `json:"origin"`
	Physical *PhysicalRef `json:"physical,omitempty"`
	Mapping  string       `json:"mapping,omitempty"`
	Enabled  bool         `json:"enabled"`
	Role     string       `json:"role,omitempty"`
	Layer    string       `json:"layer,omitempty"`
}

const (
	FactOriginSelection = "selection"
	FactOriginContext   = "context"
	FactOriginManual    = "manual"

	FactRoleAffected       = "affected"
	FactRoleHealthyControl = "healthy_control"
	FactRoleSuspected      = "suspected"
	FactRoleExcluded       = "excluded"
	FactRoleRecovered      = "recovered"

	FactMappingDeclared = "declared"
	FactMappingInferred = "inferred"
)

func (f Fact) Validate() error {
	if err := requireNonEmpty("id", f.ID); err != nil {
		return err
	}
	if err := requireNonEmpty("entity", f.Entity); err != nil {
		return err
	}
	if err := requireNonEmpty("field", f.Field); err != nil {
		return err
	}
	if err := f.Value.Validate(); err != nil {
		return err
	}
	if err := requireOneOf("origin", f.Origin, FactOriginSelection, FactOriginContext, FactOriginManual); err != nil {
		return err
	}
	if f.Physical != nil {
		if err := f.Physical.Validate(); err != nil {
			return err
		}
	}
	if f.Mapping != "" {
		if err := requireOneOf("mapping", f.Mapping, FactMappingDeclared, FactMappingInferred); err != nil {
			return err
		}
	}
	if f.Role != "" {
		if err := requireOneOf("role", f.Role, FactRoleAffected, FactRoleHealthyControl, FactRoleSuspected, FactRoleExcluded, FactRoleRecovered); err != nil {
			return err
		}
	}
	if f.Layer != "" && !validFactLayer(f.Layer) {
		return &ValidationError{Field: "layer", Message: "must be canonical or a nonempty hypothesis:, participant:, or question: overlay"}
	}
	return nil
}

func validFactLayer(layer string) bool {
	if layer == "canonical" {
		return true
	}
	for _, prefix := range []string{"hypothesis:", "participant:", "question:"} {
		if strings.HasPrefix(layer, prefix) {
			id := strings.TrimPrefix(layer, prefix)
			return id != "" && strings.TrimSpace(id) == id
		}
	}
	return false
}

// Context is the single canonical Investigation Context storage and transport
// model shared by plain investigations and incidents.
type Context struct {
	Facts []Fact `json:"facts"`
}

func (c Context) Validate() error {
	seen := make(map[string]bool, len(c.Facts))
	for index, fact := range c.Facts {
		if err := fact.Validate(); err != nil {
			return fmt.Errorf("fact %d: %w", index, err)
		}
		if seen[fact.ID] {
			return fmt.Errorf("duplicate fact id %q", fact.ID)
		}
		seen[fact.ID] = true
	}
	return nil
}
