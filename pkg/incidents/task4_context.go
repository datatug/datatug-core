package incidents

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/datatug/datatug-core/pkg/investigation"
)

// ContextFactRef identifies one fact within one named context layer. Fact IDs
// remain project-scoped; layer disambiguates the retained overlay original
// from a canonical promotion copy.
type ContextFactRef struct {
	Scope investigation.ProjectScope `json:"scope"`
	ID    string                     `json:"id"`
	Layer string                     `json:"layer"`
}

func (r ContextFactRef) Key() investigation.FactKey {
	return investigation.FactKey{Scope: r.Scope, FactID: r.ID, Layer: investigation.NormalizeFactLayer(r.Layer)}
}

func (r ContextFactRef) Validate() error {
	if err := r.Scope.ValidateFactScope(); err != nil {
		return fmt.Errorf("scope: %w", err)
	}
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("fact id is required")
	}
	if err := investigation.ValidateFactLayer(r.Layer); err != nil {
		return err
	}
	if !investigation.IsOverlayFactLayer(r.Layer) {
		return fmt.Errorf("fact layer must be an overlay")
	}
	return nil
}

func (r ContextFactRef) matches(fact investigation.Fact) bool {
	return fact.ID == r.ID && fact.Scope != nil && *fact.Scope == r.Scope &&
		investigation.NormalizeFactLayer(fact.Layer) == investigation.NormalizeFactLayer(r.Layer)
}

type ContextFactAddedPayload struct {
	Fact investigation.Fact `json:"fact"`
}

type ContextFactAddedViewPayload struct {
	Fact investigation.FactView `json:"fact"`
}

type ContextFactPromotedPayload struct {
	Fact ContextFactRef `json:"fact"`
	Role string         `json:"role"`
}

func (p ContextFactPromotedPayload) Validate() error {
	if err := p.Fact.Validate(); err != nil {
		return fmt.Errorf("fact: %w", err)
	}
	return validateCanonicalRole(p.Role)
}

type ContextFactRejectedPayload struct {
	Layer string `json:"layer"`
}

func (p ContextFactRejectedPayload) Validate() error {
	if err := investigation.ValidateFactLayer(p.Layer); err != nil {
		return err
	}
	if !investigation.IsOverlayFactLayer(p.Layer) {
		return fmt.Errorf("layer must be an overlay")
	}
	return nil
}

// ContextPromotion is durable projection history for a promotion event. The
// source fact remains in CanonicalContext under its overlay layer.
type ContextPromotion struct {
	EventID string         `json:"eventId"`
	Fact    ContextFactRef `json:"fact"`
	Role    string         `json:"role"`
}

func (p ContextPromotion) Validate() error {
	if !validSegment(p.EventID) {
		return fmt.Errorf("promotion eventId is required and canonical")
	}
	return (ContextFactPromotedPayload{Fact: p.Fact, Role: p.Role}).Validate()
}

type ContextRejection struct {
	EventID string `json:"eventId"`
	Layer   string `json:"layer"`
}

func (r ContextRejection) Validate() error {
	if !validSegment(r.EventID) {
		return fmt.Errorf("rejection eventId is required and canonical")
	}
	return (ContextFactRejectedPayload{Layer: r.Layer}).Validate()
}

func validateCanonicalRole(role string) error {
	switch role {
	case investigation.FactRoleAffected,
		investigation.FactRoleHealthyControl,
		investigation.FactRoleSuspected,
		investigation.FactRoleExcluded,
		investigation.FactRoleRecovered:
		return nil
	default:
		return fmt.Errorf("invalid canonical cohort role %q", role)
	}
}

func validateContextHypothesisRefs(refs []ArtifactRef, layer string) error {
	hypothesisID := ""
	if strings.HasPrefix(layer, "hypothesis:") {
		hypothesisID = strings.TrimPrefix(layer, "hypothesis:")
	}
	for _, ref := range refs {
		if ref.Kind != RefHypothesis {
			continue
		}
		if hypothesisID == "" || ref.ID != hypothesisID {
			return fmt.Errorf("hypothesis ref %q does not match context layer %q", ref.ID, layer)
		}
	}
	return nil
}

func contextFactIndex(facts []investigation.Fact, ref ContextFactRef) int {
	for index, fact := range facts {
		if ref.matches(fact) {
			return index
		}
	}
	return -1
}

func contextLayerHasFacts(facts []investigation.Fact, layer string) bool {
	for _, fact := range facts {
		if investigation.NormalizeFactLayer(fact.Layer) == investigation.NormalizeFactLayer(layer) {
			return true
		}
	}
	return false
}

func contextLayerRejected(rejections []ContextRejection, layer string) bool {
	for _, rejection := range rejections {
		if rejection.Layer == layer {
			return true
		}
	}
	return false
}

func cloneContextFact(fact investigation.Fact) investigation.Fact {
	cloned := fact
	if fact.Physical != nil {
		physical := *fact.Physical
		cloned.Physical = &physical
	}
	if fact.Scope != nil {
		scope := *fact.Scope
		cloned.Scope = &scope
	}
	return cloned
}

func (i Incident) validateContextHistory() error {
	seenEvents := make(map[string]bool, len(i.ContextPromotions)+len(i.ContextRejections))
	for index, promotion := range i.ContextPromotions {
		if err := promotion.Validate(); err != nil {
			return fmt.Errorf("context promotion %d: %w", index, err)
		}
		if seenEvents[promotion.EventID] {
			return fmt.Errorf("duplicate context history eventId %q", promotion.EventID)
		}
		seenEvents[promotion.EventID] = true
		sourceIndex := contextFactIndex(i.CanonicalContext.Facts, promotion.Fact)
		if sourceIndex < 0 {
			return fmt.Errorf("context promotion %d source fact is missing", index)
		}
		canonicalRef := promotion.Fact
		canonicalRef.Layer = investigation.FactLayerCanonical
		canonicalIndex := contextFactIndex(i.CanonicalContext.Facts, canonicalRef)
		if canonicalIndex < 0 {
			return fmt.Errorf("context promotion %d canonical fact is missing", index)
		}
		expected := cloneContextFact(i.CanonicalContext.Facts[sourceIndex])
		expected.Layer = investigation.FactLayerCanonical
		expected.Role = promotion.Role
		if !reflect.DeepEqual(expected, i.CanonicalContext.Facts[canonicalIndex]) {
			return fmt.Errorf("context promotion %d canonical fact differs from its overlay source", index)
		}
	}
	for index, rejection := range i.ContextRejections {
		if err := rejection.Validate(); err != nil {
			return fmt.Errorf("context overlay rejection %d: %w", index, err)
		}
		if seenEvents[rejection.EventID] {
			return fmt.Errorf("duplicate context history eventId %q", rejection.EventID)
		}
		seenEvents[rejection.EventID] = true
		if !contextLayerHasFacts(i.CanonicalContext.Facts, rejection.Layer) {
			return fmt.Errorf("context overlay rejection %d layer has no facts", index)
		}
		for prior := 0; prior < index; prior++ {
			if i.ContextRejections[prior].Layer == rejection.Layer {
				return fmt.Errorf("context overlay %q is rejected more than once", rejection.Layer)
			}
		}
	}
	return nil
}
