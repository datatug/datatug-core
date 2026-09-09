package apicontract

import (
	"encoding/json"
	"testing"
)

func validApplicableRequest() ApplicableRequest {
	return ApplicableRequest{
		Project:           "demo-project-1",
		Environment:       "local",
		SecurityContextID: "sc1",
		Values: []Fact{
			{ID: "f1", Entity: "Customer", Field: "ID", Value: NewIntegerValue("5"), Origin: FactOriginSelection, Enabled: true},
		},
	}
}

func TestApplicableRequest_JSONFieldNames(t *testing.T) {
	r := validApplicableRequest()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"project", "environment", "securityContextId", "values"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("missing key %q in %s", key, data)
		}
	}
}

func TestApplicableRequest_Validate_Valid(t *testing.T) {
	if err := validApplicableRequest().Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestApplicableRequest_Validate_EmptyValuesIsFine(t *testing.T) {
	r := validApplicableRequest()
	r.Values = nil
	if err := r.Validate(); err != nil {
		t.Errorf("expected an empty values list to be valid, got: %v", err)
	}
}

func TestApplicableRequest_Validate_RequiredScopeFields(t *testing.T) {
	missingProject := validApplicableRequest()
	missingProject.Project = ""
	if err := missingProject.Validate(); err == nil {
		t.Error("expected an error: missing project")
	}

	missingEnv := validApplicableRequest()
	missingEnv.Environment = ""
	if err := missingEnv.Validate(); err == nil {
		t.Error("expected an error: missing environment")
	}

	missingSC := validApplicableRequest()
	missingSC.SecurityContextID = ""
	if err := missingSC.Validate(); err == nil {
		t.Error("expected an error: missing securityContextId")
	}
}

func TestApplicableRequest_Validate_InvalidFactInValues(t *testing.T) {
	r := validApplicableRequest()
	r.Values = append(r.Values, Fact{Entity: "Customer", Field: "Country", Value: NewStringValue("x"), Origin: FactOriginManual, Enabled: true})
	if err := r.Validate(); err == nil {
		t.Error("expected an error: a Fact in values missing its required id")
	}
}
