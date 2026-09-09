package apicontract

import (
	"encoding/json"
	"testing"
)

func validRelatedRowsRequest() RelatedRowsRequest {
	return RelatedRowsRequest{
		Project:           "demo-project-1",
		Environment:       "local",
		SecurityContextID: "sc1",
		LookupID:          "l-invoices-by-customer",
		Value:             NewIntegerValue("5"),
	}
}

func TestRelatedRowsRequest_JSONFieldNames(t *testing.T) {
	r := validRelatedRowsRequest()
	limit := 100
	r.Limit = &limit
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"project", "environment", "securityContextId", "lookupId", "value", "limit"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("missing key %q in %s", key, data)
		}
	}
}

func TestRelatedRowsRequest_JSONFieldNames_LimitOmittedWhenAbsent(t *testing.T) {
	data, err := json.Marshal(validRelatedRowsRequest())
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	if _, ok := generic["limit"]; ok {
		t.Errorf("expected %q to be omitted when absent, got %s", "limit", data)
	}
}

func TestRelatedRowsRequest_Validate_Valid(t *testing.T) {
	if err := validRelatedRowsRequest().Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestRelatedRowsRequest_Validate_RequiredScopeFields(t *testing.T) {
	missingProject := validRelatedRowsRequest()
	missingProject.Project = ""
	if err := missingProject.Validate(); err == nil {
		t.Error("expected an error: missing project")
	}

	missingEnv := validRelatedRowsRequest()
	missingEnv.Environment = ""
	if err := missingEnv.Validate(); err == nil {
		t.Error("expected an error: missing environment")
	}

	missingSC := validRelatedRowsRequest()
	missingSC.SecurityContextID = ""
	if err := missingSC.Validate(); err == nil {
		t.Error("expected an error: missing securityContextId")
	}
}

func TestRelatedRowsRequest_Validate_LookupIDRequired(t *testing.T) {
	r := validRelatedRowsRequest()
	r.LookupID = ""
	if err := r.Validate(); err == nil {
		t.Error("expected an error: lookupId is required and opaque - an empty one cannot be revalidated")
	}
}

func TestRelatedRowsRequest_Validate_InvalidValue(t *testing.T) {
	r := validRelatedRowsRequest()
	r.Value = NewIntegerValue("not-canonical")
	if err := r.Validate(); err == nil {
		t.Error("expected an error: invalid TypedValue")
	}
}

func TestRelatedRowsRequest_Validate_Limit(t *testing.T) {
	within := validRelatedRowsRequest()
	limit := 500
	within.Limit = &limit
	if err := within.Validate(); err != nil {
		t.Errorf("expected valid at the max bound, got: %v", err)
	}

	tooHigh := validRelatedRowsRequest()
	over := 501
	tooHigh.Limit = &over
	if err := tooHigh.Validate(); err == nil {
		t.Error("expected an error: limit exceeds the maximum of 500")
	}

	zero := validRelatedRowsRequest()
	zeroLimit := 0
	zero.Limit = &zeroLimit
	if err := zero.Validate(); err == nil {
		t.Error("expected an error: limit must be positive")
	}
}
