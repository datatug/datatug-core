package dataroles

import (
	"fmt"
	"time"

	"github.com/datatug/datatug-core/pkg/reconcile"
)

// Expectation is a Meaning Graph consistency expectation.
type Expectation string

const (
	ExpectIdentical        Expectation = "identical"
	ExpectNormalized       Expectation = "normalized"
	ExpectPartial          Expectation = "partial"
	ExpectInstanceSpecific Expectation = "instance-specific"
	ExpectUnrestricted     Expectation = "unrestricted"
)

// MeaningDefault is the reusable default a Meaning Graph concept declares
// (`data-role` and `consistency`), resolved for one bound collection: Key
// holds the columns bound with role `identifier`, Properties the
// expectations of bound property concepts by column name.
type MeaningDefault struct {
	Concept     string                 `json:"concept"` // the concept's address, for provenance
	Role        Role                   `json:"role"`
	Scope       string                 `json:"scope,omitempty"` // global | application
	Expectation Expectation            `json:"expectation,omitempty"`
	Key         []string               `json:"key,omitempty"`
	Properties  map[string]Expectation `json:"properties,omitempty"`
}

// FromMeaning converts a Meaning Graph default into a classification with
// origin `imported`. It takes effect without confirmation, below a
// confirmed project classification (see Resolve), because Meaning Graph is
// an authoritative, reviewed specification rather than a suggestion.
func FromMeaning(target Target, d MeaningDefault, at time.Time) (Classification, error) {
	role := d.Role
	if role == "" {
		role = RoleUnknown
	}
	c := Classification{
		Target: target, Role: role, Origin: OriginImported, Version: d.Concept, State: StateConfirmed, At: at,
		By: "meaning-graph", Evidence: []string{fmt.Sprintf("declared by Meaning Graph concept %s", d.Concept)},
	}
	if err := c.Validate(); err != nil {
		return Classification{}, err
	}
	if d.Scope != "" && d.Scope != "global" && d.Scope != "application" {
		return Classification{}, fmt.Errorf("dataroles: unknown consistency scope %q", d.Scope)
	}
	rules, err := rulesFromExpectation(d)
	if err != nil {
		return Classification{}, err
	}
	if rules != nil && len(d.Key) == 0 {
		c.Evidence = append(c.Evidence, "the concept has no identifier binding: choose a key before reconciling")
		return c, nil
	}
	if rules != nil && d.Scope != "" {
		c.Evidence = append(c.Evidence, fmt.Sprintf("expected %s at %s scope", d.Expectation, d.Scope))
	}
	c.Consistency = rules
	return c, nil
}

func fieldPolicy(e Expectation) (reconcile.FieldPolicy, error) {
	switch e {
	case ExpectIdentical:
		return reconcile.FieldIdentical, nil
	case ExpectNormalized:
		return reconcile.FieldNormalized, nil
	case ExpectInstanceSpecific:
		return reconcile.FieldInstanceSpecific, nil
	case ExpectUnrestricted:
		return reconcile.FieldIgnored, nil
	}
	return "", fmt.Errorf("dataroles: expectation %q is not a field expectation", e)
}

func rulesFromExpectation(d MeaningDefault) (*reconcile.Rules, error) {
	rules := &reconcile.Rules{Key: append([]string(nil), d.Key...), Mode: reconcile.ModeConsensus}
	switch d.Expectation {
	case "", ExpectUnrestricted:
		if len(d.Properties) > 0 && d.Expectation == "" {
			return nil, fmt.Errorf("dataroles: property expectations need an entity expectation")
		}
		return nil, nil
	case ExpectIdentical:
		rules.Presence, rules.DefaultField = reconcile.PresenceRequired, reconcile.FieldIdentical
	case ExpectNormalized:
		rules.Presence, rules.DefaultField = reconcile.PresenceRequired, reconcile.FieldNormalized
	case ExpectPartial:
		rules.Presence, rules.DefaultField = reconcile.PresenceAny, reconcile.FieldIdentical
	case ExpectInstanceSpecific:
		rules.Presence, rules.DefaultField = reconcile.PresenceAny, reconcile.FieldInstanceSpecific
	default:
		return nil, fmt.Errorf("dataroles: unknown expectation %q", d.Expectation)
	}
	for column, expectation := range d.Properties {
		policy, err := fieldPolicy(expectation)
		if err != nil {
			return nil, err
		}
		if rules.Fields == nil {
			rules.Fields = map[string]reconcile.FieldPolicy{}
		}
		rules.Fields[column] = policy
	}
	return rules, nil
}

// Resolve returns the effective classification for a target, most specific
// first: the project's confirmed classification, then the Meaning Graph
// default when one is given. A profile's own rule outranks both and is
// applied by the caller. Suggestions never take effect.
func Resolve(doc Document, target Target, meaning *Classification) (Classification, bool) {
	if c, ok := doc.Effective(target); ok {
		return c, true
	}
	if meaning != nil && meaning.State == StateConfirmed && meaning.Target == target {
		return *meaning, true
	}
	return Classification{}, false
}
