package apicontract

import (
	"encoding/json"
	"testing"
)

func TestFact_JSONFieldNames(t *testing.T) {
	f := Fact{
		ID:      "f1",
		Entity:  "Customer",
		Field:   "ID",
		Value:   NewIntegerValue("5"),
		Origin:  "selection",
		Mapping: "declared",
		Enabled: true,
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"f1","entity":"Customer","field":"ID","value":{"type":"integer","value":"5"},"origin":"selection","mapping":"declared","enabled":true}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestFact_PhysicalAndMappingOmittedWhenAbsent(t *testing.T) {
	f := Fact{ID: "f1", Entity: "Customer", Field: "ID", Value: NewIntegerValue("5"), Origin: "manual", Enabled: false}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"f1","entity":"Customer","field":"ID","value":{"type":"integer","value":"5"},"origin":"manual","enabled":false}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestFact_PhysicalPresent(t *testing.T) {
	phys := PhysicalRef{Source: "chinook", Collection: "Customer", Column: "CustomerId"}
	f := Fact{ID: "f1", Entity: "Customer", Field: "ID", Value: NewIntegerValue("5"), Origin: "selection", Physical: &phys, Enabled: true}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	var back Fact
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Physical == nil || *back.Physical != phys {
		t.Errorf("physical round-trip failed: %+v", back.Physical)
	}
}

func TestFact_Validate(t *testing.T) {
	valid := Fact{ID: "f1", Entity: "Customer", Field: "ID", Value: NewIntegerValue("5"), Origin: "selection", Enabled: true}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	missingID := valid
	missingID.ID = ""
	if err := missingID.Validate(); err == nil {
		t.Error("expected an error: missing id")
	}

	missingEntity := valid
	missingEntity.Entity = ""
	if err := missingEntity.Validate(); err == nil {
		t.Error("expected an error: missing entity")
	}

	missingField := valid
	missingField.Field = ""
	if err := missingField.Validate(); err == nil {
		t.Error("expected an error: missing field")
	}

	badOrigin := valid
	badOrigin.Origin = "bogus"
	if err := badOrigin.Validate(); err == nil {
		t.Error("expected an error: invalid origin")
	}

	badValue := valid
	badValue.Value = NewIntegerValue("007")
	if err := badValue.Validate(); err == nil {
		t.Error("expected an error: invalid value")
	}

	badMapping := valid
	badMapping.Mapping = "guessed"
	if err := badMapping.Validate(); err == nil {
		t.Error("expected an error: invalid mapping")
	}

	validMapping := valid
	validMapping.Mapping = "inferred"
	if err := validMapping.Validate(); err != nil {
		t.Errorf("expected 'inferred' mapping to be valid, got: %v", err)
	}

	badPhysical := valid
	badPhysical.Physical = &PhysicalRef{Source: "chinook"}
	if err := badPhysical.Validate(); err == nil {
		t.Error("expected an error: invalid physical ref")
	}
}

func TestFact_EnabledFalseAndOriginManualAreValidPresentValues(t *testing.T) {
	f := Fact{ID: "f1", Entity: "Customer", Field: "ID", Value: NewBooleanValue(false), Origin: "context", Enabled: false}
	if err := f.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}
