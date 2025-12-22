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
	if err := ValidateName("valid"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := ValidateName("not valid"); err == nil {
		t.Error("expected an error for name with space")
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
	if err := ValidateTitle("Valid Title"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := ValidateTitle(" Title "); err == nil {
		t.Error("expected error for title with leading/trailing spaces")
	}
}
