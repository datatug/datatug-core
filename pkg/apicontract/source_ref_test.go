package apicontract

import (
	"encoding/json"
	"testing"
)

func TestSourceRef_JSONFieldNames(t *testing.T) {
	r := SourceRef{Source: "chinook", Collection: "Customer"}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"source":"chinook","collection":"Customer"}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestSourceRef_Validate(t *testing.T) {
	if err := (SourceRef{Source: "chinook", Collection: "Customer"}).Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
	if err := (SourceRef{Collection: "Customer"}).Validate(); err == nil {
		t.Error("expected an error: missing source")
	}
	if err := (SourceRef{Source: "chinook"}).Validate(); err == nil {
		t.Error("expected an error: missing collection")
	}
}
