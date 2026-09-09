package apicontract

import (
	"encoding/json"
	"testing"
)

func TestLimitation_JSONFieldNames(t *testing.T) {
	l := Limitation{Policy: "support/customers-support", RowsFiltered: true, HiddenColumns: []string{"Email"}}
	data, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"policy":"support/customers-support","rowsFiltered":true,"hiddenColumns":["Email"]}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestLimitation_EmptyHiddenColumnsMarshalsAsEmptyArray(t *testing.T) {
	l := Limitation{Policy: "generic", RowsFiltered: false, HiddenColumns: []string{}}
	data, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"policy":"generic","rowsFiltered":false,"hiddenColumns":[]}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestLimitation_Validate(t *testing.T) {
	if err := (Limitation{Policy: "p", HiddenColumns: []string{}}).Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
	if err := (Limitation{HiddenColumns: []string{}}).Validate(); err == nil {
		t.Error("expected an error: missing policy")
	}
}
