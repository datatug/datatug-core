package apicontract

import (
	"encoding/json"
	"testing"
)

func TestErrorEnvelope_JSONFieldNames(t *testing.T) {
	e := ErrorEnvelope{Error: ErrorBody{
		Code: string(ErrCodeMissingParameter), Message: "CustomerId is required", Field: "CustomerId", RequestID: "req-1",
	}}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"error":{"code":"MISSING_PARAMETER","message":"CustomerId is required","field":"CustomerId","requestId":"req-1"}}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestErrorEnvelope_FieldAndTargetsOmittedWhenAbsent(t *testing.T) {
	e := ErrorEnvelope{Error: ErrorBody{Code: string(ErrCodeAccessDenied), Message: "denied", RequestID: "req-1"}}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"error":{"code":"ACCESS_DENIED","message":"denied","requestId":"req-1"}}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestErrorEnvelope_TargetsOnTargetRequired(t *testing.T) {
	e := ErrorEnvelope{Error: ErrorBody{
		Code: string(ErrCodeTargetRequired), Message: "choose a source", RequestID: "req-1",
		Targets: []TargetOption{{Source: "chinook-local", Label: "Local"}, {Source: "chinook-prod", Label: "Prod"}},
	}}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"error":{"code":"TARGET_REQUIRED","message":"choose a source","requestId":"req-1","targets":[{"source":"chinook-local","label":"Local"},{"source":"chinook-prod","label":"Prod"}]}}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestErrorCode_HTTPStatus(t *testing.T) {
	cases := []struct {
		code ErrorCode
		want int
	}{
		{ErrCodeInvalidRequest, 400},
		{ErrCodeTypeMismatch, 400},
		{ErrCodeMissingParameter, 400},
		{ErrCodeAmbiguousBinding, 400},
		{ErrCodeTargetRequired, 400},
		{ErrCodeUnauthenticated, 401},
		{ErrCodeAccessDenied, 403},
		{ErrCodeUnsupportedProtectedExecution, 403},
		{ErrCodeNotFound, 404},
		{ErrCodeStaleContext, 409},
		{ErrCodeResponseTooLarge, 413},
		{ErrCodeSourceUnavailable, 503},
		{ErrCodeTimeout, 504},
	}
	for _, c := range cases {
		t.Run(string(c.code), func(t *testing.T) {
			if got := c.code.HTTPStatus(); got != c.want {
				t.Errorf("%s.HTTPStatus() = %d, want %d", c.code, got, c.want)
			}
			if !c.code.Valid() {
				t.Errorf("%s.Valid() = false, want true", c.code)
			}
		})
	}
}

func TestErrorCode_UnknownCodeInvalid(t *testing.T) {
	unknown := ErrorCode("BOGUS")
	if unknown.Valid() {
		t.Error("expected an unknown error code to be invalid")
	}
	if got := unknown.HTTPStatus(); got != 0 {
		t.Errorf("expected HTTPStatus() 0 for an unknown code, got %d", got)
	}
}

func TestErrorBody_Validate(t *testing.T) {
	valid := ErrorBody{Code: string(ErrCodeAccessDenied), Message: "denied", RequestID: "req-1"}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	unknownCode := ErrorBody{Code: "BOGUS", Message: "x", RequestID: "req-1"}
	if err := unknownCode.Validate(); err == nil {
		t.Error("expected an error: unknown error code")
	}

	missingMessage := ErrorBody{Code: string(ErrCodeAccessDenied), RequestID: "req-1"}
	if err := missingMessage.Validate(); err == nil {
		t.Error("expected an error: missing message")
	}

	missingRequestID := ErrorBody{Code: string(ErrCodeAccessDenied), Message: "denied"}
	if err := missingRequestID.Validate(); err == nil {
		t.Error("expected an error: missing requestId")
	}
}

func TestTargetOption_Validate(t *testing.T) {
	if err := (TargetOption{Source: "chinook-local", Label: "Local"}).Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
	if err := (TargetOption{Label: "Local"}).Validate(); err == nil {
		t.Error("expected an error: missing source")
	}
	if err := (TargetOption{Source: "chinook-local"}).Validate(); err == nil {
		t.Error("expected an error: missing label")
	}
}

func TestErrorBody_Validate_InvalidTargetEntry(t *testing.T) {
	e := ErrorBody{
		Code: string(ErrCodeTargetRequired), Message: "choose a source", RequestID: "req-1",
		Targets: []TargetOption{{Source: "chinook-local"}}, // missing label
	}
	if err := e.Validate(); err == nil {
		t.Error("expected an error: invalid target entry")
	}
}

func TestErrorEnvelope_Validate(t *testing.T) {
	valid := ErrorEnvelope{Error: ErrorBody{Code: string(ErrCodeAccessDenied), Message: "denied", RequestID: "req-1"}}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	invalid := ErrorEnvelope{Error: ErrorBody{Code: "BOGUS", Message: "denied", RequestID: "req-1"}}
	if err := invalid.Validate(); err == nil {
		t.Error("expected an error: invalid error body")
	}
}

func TestErrorBody_Validate_TargetsOnlyOnTargetRequired(t *testing.T) {
	// "Only TARGET_REQUIRED may include authorized target options."
	misplaced := ErrorBody{
		Code: string(ErrCodeAccessDenied), Message: "denied", RequestID: "req-1",
		Targets: []TargetOption{{Source: "chinook-local", Label: "Local"}},
	}
	if err := misplaced.Validate(); err == nil {
		t.Error("expected an error: targets on a non-TARGET_REQUIRED error")
	}

	onTargetRequired := ErrorBody{
		Code: string(ErrCodeTargetRequired), Message: "choose a source", RequestID: "req-1",
		Targets: []TargetOption{{Source: "chinook-local", Label: "Local"}},
	}
	if err := onTargetRequired.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}
