package apicontract

// Binding is one parameter's resolved value and its provenance. Returned
// bindings are execution-confirmed; a selection/context/manual origin's
// OriginEvidence remains explicitly "client-reported", never presented as
// server-attested; "server-default" is used only when a declared default was
// validated against the query definition. api-contract.md "Shared JSON
// types" / "Endpoint table".
type Binding struct {
	ParameterID    string     `json:"parameterId"`
	Value          TypedValue `json:"value"`
	Origin         string     `json:"origin"`         // selection | context | manual | default
	OriginEvidence string     `json:"originEvidence"` // server-default | client-reported
	FactID         string     `json:"factId,omitempty"`
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
	if err := requireNonEmpty("parameterId", b.ParameterID); err != nil {
		return err
	}
	if err := b.Value.Validate(); err != nil {
		return err
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
	return nil
}
