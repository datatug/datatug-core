package apicontract

import "strings"

// Fact is a semantic suggestion, not an access credential - the server
// revalidates physical references and mapping declarations against
// authorized project metadata. Manual facts cannot impersonate a selected
// protected record. api-contract.md "Shared JSON types".
type Fact struct {
	ID       string       `json:"id"`
	Entity   string       `json:"entity"`
	Field    string       `json:"field"`
	Value    TypedValue   `json:"value"`
	Origin   string       `json:"origin"` // selection | context | manual
	Physical *PhysicalRef `json:"physical,omitempty"`
	Mapping  string       `json:"mapping,omitempty"` // declared | inferred
	Enabled  bool         `json:"enabled"`
	Role     string       `json:"role,omitempty"`  // affected | healthy_control | suspected | excluded | recovered
	Layer    string       `json:"layer,omitempty"` // canonical or an overlay id
}

const (
	FactOriginSelection = "selection"
	FactOriginContext   = "context"
	FactOriginManual    = "manual"
)

const (
	FactRoleAffected       = "affected"
	FactRoleHealthyControl = "healthy_control"
	FactRoleSuspected      = "suspected"
	FactRoleExcluded       = "excluded"
	FactRoleRecovered      = "recovered"
)

const (
	FactMappingDeclared = "declared"
	FactMappingInferred = "inferred"
)

// Validate enforces id/entity/field are required, Value is itself valid,
// Origin is one of the closed set, Physical (when present) is itself valid,
// and Mapping (when present) is one of the closed set.
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
