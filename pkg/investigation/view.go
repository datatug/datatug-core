package investigation

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// RedactionMarker replaces a fact's TypedValue only in a current-policy read
// view. It is never valid in Context, Fact, or stored incident events.
type RedactionMarker struct {
	Redacted bool `json:"redacted"`
}

// ValueView is exactly one canonical TypedValue or one redaction marker.
type ValueView struct {
	Value    *TypedValue
	Redacted bool
}

func VisibleValue(value TypedValue) ValueView { return ValueView{Value: &value} }
func RedactedValue() ValueView                { return ValueView{Redacted: true} }

func (v ValueView) Validate() error {
	if v.Value != nil && !v.Redacted {
		return v.Value.Validate()
	}
	if v.Value == nil && v.Redacted {
		return nil
	}
	return fmt.Errorf("fact view value must be exactly one typed value or redaction marker")
}

func (v ValueView) MarshalJSON() ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	if v.Redacted {
		return json.Marshal(RedactionMarker{Redacted: true})
	}
	return json.Marshal(v.Value)
}

func (v *ValueView) UnmarshalJSON(data []byte) error {
	var marker map[string]json.RawMessage
	if err := json.Unmarshal(data, &marker); err != nil {
		return err
	}
	if raw, ok := marker["redacted"]; ok {
		if len(marker) != 1 || !bytes.Equal(bytes.TrimSpace(raw), []byte("true")) {
			return fmt.Errorf("redaction marker must be exactly {\"redacted\":true}")
		}
		*v = RedactedValue()
		return nil
	}
	var value TypedValue
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*v = VisibleValue(value)
	return nil
}

// FactView is a read-only projection of the canonical Fact. It keeps the
// existing fact JSON shape when visible and substitutes only value when the
// serving adapter's current policy says it must be redacted.
type FactView struct {
	ID       string        `json:"id"`
	Entity   string        `json:"entity"`
	Field    string        `json:"field,omitempty"`
	Value    ValueView     `json:"value"`
	Origin   string        `json:"origin"`
	Physical *PhysicalRef  `json:"physical,omitempty"`
	Mapping  string        `json:"mapping,omitempty"`
	Enabled  bool          `json:"enabled"`
	Role     string        `json:"role,omitempty"`
	Layer    string        `json:"layer,omitempty"`
	Scope    *ProjectScope `json:"scope,omitempty"`
}

func VisibleFact(fact Fact) FactView {
	return FactView{
		ID: fact.ID, Entity: fact.Entity, Field: fact.Field, Value: VisibleValue(fact.Value),
		Origin: fact.Origin, Physical: clonePhysicalRef(fact.Physical), Mapping: fact.Mapping, Enabled: fact.Enabled,
		Role: fact.Role, Layer: fact.Layer, Scope: cloneProjectScope(fact.Scope),
	}
}

func RedactedFact(fact Fact, includeField bool) FactView {
	field := ""
	if includeField {
		field = fact.Field
	}
	return FactView{
		ID: fact.ID, Entity: fact.Entity, Field: field, Value: RedactedValue(),
		Origin: fact.Origin, Enabled: fact.Enabled, Role: fact.Role, Layer: fact.Layer,
		Scope: cloneProjectScope(fact.Scope),
	}
}

func clonePhysicalRef(value *PhysicalRef) *PhysicalRef {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneProjectScope(value *ProjectScope) *ProjectScope {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func (f FactView) Validate() error {
	if err := requireNonEmpty("id", f.ID); err != nil {
		return err
	}
	if err := requireNonEmpty("entity", f.Entity); err != nil {
		return err
	}
	if err := f.Value.Validate(); err != nil {
		return err
	}
	if !f.Redacted() {
		if err := requireNonEmpty("field", f.Field); err != nil {
			return err
		}
		canonical := Fact{
			ID: f.ID, Entity: f.Entity, Field: f.Field, Value: *f.Value.Value,
			Origin: f.Origin, Physical: f.Physical, Mapping: f.Mapping, Enabled: f.Enabled,
			Role: f.Role, Layer: f.Layer, Scope: f.Scope,
		}
		return canonical.Validate()
	}
	if err := requireOneOf("origin", f.Origin, FactOriginSelection, FactOriginContext, FactOriginManual); err != nil {
		return err
	}
	if f.Physical != nil || f.Mapping != "" {
		return fmt.Errorf("redacted fact view cannot expose physical mapping")
	}
	if f.Role != "" {
		if err := requireOneOf("role", f.Role, FactRoleAffected, FactRoleHealthyControl, FactRoleSuspected, FactRoleExcluded, FactRoleRecovered); err != nil {
			return err
		}
	}
	if f.Layer != "" && !validFactLayer(f.Layer) {
		return &ValidationError{Field: "layer", Message: "must be canonical or a nonempty hypothesis:, participant:, or question: overlay"}
	}
	if f.Scope != nil {
		if err := f.Scope.Validate(); err != nil {
			return &ValidationError{Field: "scope", Message: err.Error()}
		}
	}
	return nil
}

func (f FactView) Redacted() bool { return f.Value.Redacted }

type ContextView struct {
	Facts []FactView `json:"facts"`
}

func VisibleContext(context Context) ContextView {
	facts := make([]FactView, len(context.Facts))
	for index, fact := range context.Facts {
		facts[index] = VisibleFact(fact)
	}
	return ContextView{Facts: facts}
}

func (c ContextView) Validate() error {
	seen := make(map[FactKey]bool, len(c.Facts))
	for index, fact := range c.Facts {
		if err := fact.Validate(); err != nil {
			return fmt.Errorf("fact %d: %w", index, err)
		}
		key := FactKey{FactID: fact.ID}
		if fact.Scope != nil {
			key.Scope = *fact.Scope
		}
		if seen[key] {
			return fmt.Errorf("duplicate fact id %q in project scope", fact.ID)
		}
		seen[key] = true
	}
	return nil
}
