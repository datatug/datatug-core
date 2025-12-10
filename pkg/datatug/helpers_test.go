package datatug

import "testing"

func TestValidateName(t *testing.T) {
	if err := ValidateName(""); err == nil {
		t.Error("expected an error got nil")
	}
	if err := ValidateName(" "); err == nil {
		t.Error("expected an error got nil")
	}
	if err := ValidateName("\t"); err == nil {
		t.Error("expected an error got nil")
	}
}

func TestValidateTitle(t *testing.T) {
	if err := ValidateTitle(""); err == nil {
		t.Error("expected an error got nil")
	}
	if err := ValidateTitle(" "); err == nil {
		t.Error("expected an error got nil")
	}
	if err := ValidateTitle("\t"); err == nil {
		t.Error("expected an error got nil")
	}
}
