package apicontract

import (
	"encoding/json"
	"testing"
)

func TestPhysicalRef_JSONFieldNames(t *testing.T) {
	r := PhysicalRef{Source: "chinook", Collection: "Customer", Column: "CustomerId"}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"source":"chinook","collection":"Customer","column":"CustomerId"}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestPhysicalRef_Validate(t *testing.T) {
	if err := (PhysicalRef{Source: "chinook", Collection: "Customer", Column: "CustomerId"}).Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
	cases := []PhysicalRef{
		{Collection: "Customer", Column: "CustomerId"},
		{Source: "chinook", Column: "CustomerId"},
		{Source: "chinook", Collection: "Customer"},
	}
	for _, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("expected an error for %+v", c)
		}
	}
}
