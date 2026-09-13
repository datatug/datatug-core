package investigation

import (
	"fmt"
	"strings"
	"unicode"
)

// ProjectScope is persisted provenance for a fact or cross-project reference.
// It deliberately excludes the request-only securityContextId.
type ProjectScope struct {
	StoreID     string `json:"storeId"`
	ProjectID   string `json:"projectId"`
	Environment string `json:"environment,omitempty"`
}

func (s ProjectScope) Validate() error {
	if !validScopeSegment(s.StoreID) {
		return fmt.Errorf("invalid project storeId %q", s.StoreID)
	}
	if !validScopeSegment(s.ProjectID) {
		return fmt.Errorf("projectId is required")
	}
	return nil
}

// ValidateFactScope strengthens the legacy ProjectRef-compatible validation
// with the explicit canonical environment required for newly attached facts.
func (s ProjectScope) ValidateFactScope() error {
	if err := s.Validate(); err != nil {
		return err
	}
	if !validScopeSegment(s.Environment) {
		return fmt.Errorf("environment is required and canonical")
	}
	return nil
}

func validScopeSegment(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

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
	ID        string        `json:"id"`
	Entity    string        `json:"entity"`
	Field     string        `json:"field"`
	Value     TypedValue    `json:"value"`
	Condition string        `json:"condition,omitempty"`
	Origin    string        `json:"origin"`
	Physical  *PhysicalRef  `json:"physical,omitempty"`
	Mapping   string        `json:"mapping,omitempty"`
	Enabled   bool          `json:"enabled"`
	Role      string        `json:"role,omitempty"`
	Layer     string        `json:"layer,omitempty"`
	Scope     *ProjectScope `json:"scope,omitempty"`
}

// FactKey is the comparable, server-qualified and layer-qualified identity
// policy adapters use. The layer distinguishes a retained overlay original
// from its canonical promotion copy.
type FactKey struct {
	Scope  ProjectScope
	FactID string
	Layer  string
}

func (f Fact) Key() FactKey {
	key := FactKey{FactID: f.ID, Layer: NormalizeFactLayer(f.Layer)}
	if f.Scope != nil {
		key.Scope = *f.Scope
	}
	return key
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

	FactLayerCanonical = "canonical"

	FactMappingDeclared = "declared"
	FactMappingInferred = "inferred"

	// An omitted condition has the canonical equality meaning. Non-default
	// predicates must survive transport and persistence explicitly so a caller
	// can never silently turn `Customer.ID > 5` into equality.
	FactConditionEqual              = "=="
	FactConditionNotEqual           = "!="
	FactConditionGreaterThan        = ">"
	FactConditionGreaterThanOrEqual = ">="
	FactConditionLessThan           = "<"
	FactConditionLessThanOrEqual    = "<="
)

// NormalizeFactLayer preserves the transport compatibility rule that an
// omitted layer means canonical.
func NormalizeFactLayer(layer string) string {
	if layer == "" {
		return FactLayerCanonical
	}
	return layer
}

// ValidateFactLayer owns the shared layer vocabulary for every context
// consumer. An omitted layer remains valid as the legacy canonical spelling.
func ValidateFactLayer(layer string) error {
	if layer == "" || layer == FactLayerCanonical {
		return nil
	}
	for _, prefix := range []string{"hypothesis:", "participant:", "question:"} {
		if strings.HasPrefix(layer, prefix) {
			id := strings.TrimPrefix(layer, prefix)
			if validFactLayerOwner(id) {
				return nil
			}
		}
	}
	return &ValidationError{Field: "layer", Message: "must be canonical or a nonempty hypothesis:, participant:, or question: overlay"}
}

func validFactLayerOwner(id string) bool {
	if id == "" || strings.TrimSpace(id) != id {
		return false
	}
	for _, r := range id {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func IsOverlayFactLayer(layer string) bool {
	return layer != "" && layer != FactLayerCanonical && ValidateFactLayer(layer) == nil
}

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
	if f.Condition != "" {
		if err := requireOneOf("condition", f.Condition,
			FactConditionEqual, FactConditionNotEqual,
			FactConditionGreaterThan, FactConditionGreaterThanOrEqual,
			FactConditionLessThan, FactConditionLessThanOrEqual,
		); err != nil {
			return err
		}
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
	if err := ValidateFactLayer(f.Layer); err != nil {
		return err
	}
	if f.Scope != nil {
		if err := f.Scope.Validate(); err != nil {
			return &ValidationError{Field: "scope", Message: err.Error()}
		}
	}
	return nil
}

// Context is the single canonical Investigation Context storage and transport
// model shared by plain investigations and incidents.
type Context struct {
	Facts []Fact `json:"facts"`
}

func (c Context) Validate() error {
	seen := make(map[FactKey]bool, len(c.Facts))
	for index, fact := range c.Facts {
		if err := fact.Validate(); err != nil {
			return fmt.Errorf("fact %d: %w", index, err)
		}
		key := fact.Key()
		if seen[key] {
			return fmt.Errorf("duplicate fact id %q in project scope", fact.ID)
		}
		seen[key] = true
	}
	return nil
}

// ValidateScoped is the creation boundary for new persisted contexts. Legacy
// contexts without scope remain readable through Validate, but new facts must
// name an explicit server store, project, and environment.
func (c Context) ValidateScoped() error {
	if err := c.Validate(); err != nil {
		return err
	}
	for index, fact := range c.Facts {
		if fact.Scope == nil {
			return fmt.Errorf("fact %d: scope with environment is required", index)
		}
		if err := fact.Scope.ValidateFactScope(); err != nil {
			return fmt.Errorf("fact %d: scope: %w", index, err)
		}
	}
	return nil
}

// ValidateAllowedScopes binds new facts to the request's primary project or
// one explicitly declared secondary. Authorization is still performed by the
// serving adapter; this helper only prevents invented provenance.
func (c Context) ValidateAllowedScopes(primary ProjectScope, declared []ProjectScope) error {
	if err := c.ValidateScoped(); err != nil {
		return err
	}
	if err := primary.ValidateFactScope(); err != nil {
		return fmt.Errorf("primary project: %w", err)
	}
	for index, fact := range c.Facts {
		if *fact.Scope == primary || containsProjectScope(declared, *fact.Scope) {
			continue
		}
		return fmt.Errorf("fact %d: scope is neither the primary project nor a declared secondary", index)
	}
	return nil
}

func containsProjectScope(scopes []ProjectScope, wanted ProjectScope) bool {
	for _, scope := range scopes {
		if scope == wanted {
			return true
		}
	}
	return false
}
