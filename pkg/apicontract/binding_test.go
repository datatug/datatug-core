package apicontract

import (
	"encoding/json"
	"testing"
)

func TestBinding_JSONFieldNames(t *testing.T) {
	b := Binding{
		ParameterID:    "CustomerId",
		Value:          NewIntegerValue("5"),
		Origin:         "selection",
		OriginEvidence: "client-reported",
		FactID:         "f1",
	}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"parameterId":"CustomerId","value":{"type":"integer","value":"5"},"origin":"selection","originEvidence":"client-reported","factId":"f1"}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestBinding_FactIDOmittedWhenAbsent(t *testing.T) {
	b := Binding{ParameterID: "CustomerId", Value: NewIntegerValue("5"), Origin: "default", OriginEvidence: "server-default"}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"parameterId":"CustomerId","value":{"type":"integer","value":"5"},"origin":"default","originEvidence":"server-default"}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestBinding_Validate(t *testing.T) {
	valid := Binding{ParameterID: "CustomerId", Value: NewIntegerValue("5"), Origin: "selection", OriginEvidence: "client-reported"}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	validDefault := Binding{ParameterID: "CustomerId", Value: NewIntegerValue("5"), Origin: "default", OriginEvidence: "server-default"}
	if err := validDefault.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	missingParam := valid
	missingParam.ParameterID = ""
	if err := missingParam.Validate(); err == nil {
		t.Error("expected an error: missing parameterId")
	}

	badOrigin := valid
	badOrigin.Origin = "bogus"
	if err := badOrigin.Validate(); err == nil {
		t.Error("expected an error: invalid origin")
	}

	badEvidence := valid
	badEvidence.OriginEvidence = "bogus"
	if err := badEvidence.Validate(); err == nil {
		t.Error("expected an error: invalid originEvidence")
	}
}

func TestBinding_Validate_InvalidValue(t *testing.T) {
	b := Binding{ParameterID: "p", Value: NewIntegerValue("not-canonical"), Origin: "selection", OriginEvidence: "client-reported"}
	if err := b.Validate(); err == nil {
		t.Error("expected an error: invalid value")
	}
}

func TestBinding_ServerDefaultOriginEvidencePairing(t *testing.T) {
	// "Server default origins use originEvidence:server-default only when
	// validated against the query definition" - and conversely a
	// selection/context/manual origin's binding "remains explicitly
	// client-reported" - so the two fields cannot be mismatched.
	mismatched := Binding{ParameterID: "p", Value: NewBooleanValue(true), Origin: "default", OriginEvidence: "client-reported"}
	if err := mismatched.Validate(); err == nil {
		t.Error("expected an error: origin=default must pair with originEvidence=server-default")
	}

	mismatched2 := Binding{ParameterID: "p", Value: NewBooleanValue(true), Origin: "selection", OriginEvidence: "server-default"}
	if err := mismatched2.Validate(); err == nil {
		t.Error("expected an error: a client origin must not be labeled server-default")
	}
}
