package datatug

import "fmt"

// EnvironmentKind is the closed set of environment kinds from the semantic
// discovery spec. A local access-policy rule may select it with
// `environment: <kind>` (pkg/policyscope); a rule without that selector is
// unaffected by kind.
type EnvironmentKind string

const (
	EnvironmentKindDevelopment EnvironmentKind = "development"
	EnvironmentKindTest        EnvironmentKind = "test"
	EnvironmentKindUAT         EnvironmentKind = "uat"
	EnvironmentKindStaging     EnvironmentKind = "staging"
	EnvironmentKindProduction  EnvironmentKind = "production"
	EnvironmentKindDemo        EnvironmentKind = "demo"
	EnvironmentKindCustom      EnvironmentKind = "custom"
	EnvironmentKindUnknown     EnvironmentKind = "unknown"
)

// EnvironmentKinds lists every valid kind in display order.
var EnvironmentKinds = []EnvironmentKind{
	EnvironmentKindDevelopment, EnvironmentKindTest, EnvironmentKindUAT, EnvironmentKindStaging,
	EnvironmentKindProduction, EnvironmentKindDemo, EnvironmentKindCustom, EnvironmentKindUnknown,
}

// Validate accepts the empty kind (unset) and every member of the set.
func (k EnvironmentKind) Validate() error {
	if k == "" {
		return nil
	}
	for _, kind := range EnvironmentKinds {
		if k == kind {
			return nil
		}
	}
	return fmt.Errorf("unknown environment kind %q", string(k))
}
