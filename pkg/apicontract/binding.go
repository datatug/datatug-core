package apicontract

import (
	"fmt"
	"strings"
)

// Binding is one parameter's resolved value and its provenance. Returned
// bindings are execution-confirmed; a selection/context/manual origin's
// OriginEvidence remains explicitly "client-reported", never presented as
// server-attested; "server-default" is used only when a declared default was
// validated against the query definition. api-contract.md "Shared JSON
// types" / "Endpoint table".
type Binding struct {
	ParameterID    string          `json:"parameterId"`
	Value          TypedValueOrSet `json:"value"`
	Origin         string          `json:"origin"`         // selection | context | manual | default
	OriginEvidence string          `json:"originEvidence"` // server-default | client-reported
	FactID         string          `json:"factId,omitempty"`
	ValueFactIDs   [][]string      `json:"valueFactIds,omitempty"`
}

const (
	BindingOriginSelection = "selection"
	BindingOriginContext   = "context"
	BindingOriginManual    = "manual"
	BindingOriginDefault   = "default"
)

const (
	BindingOriginEvidenceServerDefault  = "server-default"
	BindingOriginEvidenceClientReported = "client-reported"
)

// Validate enforces ParameterID is required, Value is itself valid, Origin
// and OriginEvidence are each one of their closed sets, and the two pair
// correctly: Origin "default" only with OriginEvidence "server-default",
// every other Origin only with "client-reported" - "The UI must not present
// client origins as server-attested provenance."
func (b Binding) Validate() error {
	if err := requireCanonicalString("parameterId", b.ParameterID); err != nil {
		return err
	}
	if err := b.Value.Validate(); err != nil {
		return &ValidationError{Field: "value", Message: err.Error()}
	}
	if err := requireOneOf("origin", b.Origin, BindingOriginSelection, BindingOriginContext, BindingOriginManual, BindingOriginDefault); err != nil {
		return err
	}
	if err := requireOneOf("originEvidence", b.OriginEvidence, BindingOriginEvidenceServerDefault, BindingOriginEvidenceClientReported); err != nil {
		return err
	}
	wantEvidence := BindingOriginEvidenceClientReported
	if b.Origin == BindingOriginDefault {
		wantEvidence = BindingOriginEvidenceServerDefault
	}
	if b.OriginEvidence != wantEvidence {
		return &ValidationError{Field: "originEvidence", Message: "origin " + b.Origin + " must pair with originEvidence " + wantEvidence}
	}
	return validateValueFactProvenance(b.Origin, b.Value, b.FactID, b.ValueFactIDs)
}

func validateValueFactProvenance(origin string, value TypedValueOrSet, factID string, valueFactIDs [][]string) error {
	if factID != "" && valueFactIDs != nil {
		return &ValidationError{Field: "factId/valueFactIds", Message: "must not both be present"}
	}
	fromFacts := origin == BindingOriginSelection || origin == BindingOriginContext
	if value.IsSet() {
		if factID != "" {
			return &ValidationError{Field: "factId", Message: "must be absent for a set value"}
		}
		if !fromFacts {
			if valueFactIDs != nil {
				return &ValidationError{Field: "valueFactIds", Message: "manual and default values must not claim fact provenance"}
			}
			return nil
		}
		if valueFactIDs == nil || len(valueFactIDs) != len(value.Set.Values) {
			return &ValidationError{Field: "valueFactIds", Message: "must contain one non-empty fact-id group per canonical set value"}
		}
		return validateFactIDGroups(valueFactIDs)
	}
	if valueFactIDs != nil {
		return &ValidationError{Field: "valueFactIds", Message: "must be absent for a scalar value"}
	}
	if fromFacts {
		return validateFactID("factId", factID)
	}
	if factID != "" {
		return &ValidationError{Field: "factId", Message: "manual and default values must not claim fact provenance"}
	}
	return nil
}

func validateFactIDGroups(groups [][]string) error {
	seen := make(map[string]struct{})
	for i, group := range groups {
		if len(group) == 0 {
			return &ValidationError{Field: "valueFactIds", Message: fmt.Sprintf("group %d must not be empty", i)}
		}
		previous := ""
		for j, factID := range group {
			if err := validateFactID("valueFactIds", factID); err != nil {
				return &ValidationError{Field: "valueFactIds", Message: fmt.Sprintf("group %d index %d: %s", i, j, err)}
			}
			if j > 0 && strings.Compare(previous, factID) >= 0 {
				return &ValidationError{Field: "valueFactIds", Message: fmt.Sprintf("group %d must be sorted and deduplicated", i)}
			}
			if _, ok := seen[factID]; ok {
				return &ValidationError{Field: "valueFactIds", Message: "one fact id must not be attributed to different canonical values"}
			}
			seen[factID] = struct{}{}
			previous = factID
		}
	}
	return nil
}

func validateFactID(field, value string) error {
	if err := requireNonEmpty(field, value); err != nil {
		return err
	}
	if strings.TrimSpace(value) != value {
		return &ValidationError{Field: field, Message: "must be canonical"}
	}
	return nil
}
