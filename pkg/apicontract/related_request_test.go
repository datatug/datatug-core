package apicontract

import (
	"encoding/json"
	"testing"
)

func validRelatedRequest() RelatedRequest {
	return RelatedRequest{
		Project:           "demo-project-1",
		Environment:       "local",
		SecurityContextID: "sc1",
		Fact:              Fact{ID: "f1", Entity: "Customer", Field: "ID", Value: NewIntegerValue("5"), Origin: FactOriginSelection, Enabled: true},
	}
}

func TestRelatedRequest_JSONFieldNames(t *testing.T) {
	r := validRelatedRequest()
	limit := 10
	r.Limit = &limit
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"project", "environment", "securityContextId", "fact", "limit"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("missing key %q in %s", key, data)
		}
	}
}

func TestRelatedRequest_JSONFieldNames_LimitOmittedWhenAbsent(t *testing.T) {
	data, err := json.Marshal(validRelatedRequest())
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

func TestRelatedRequest_Validate_Valid(t *testing.T) {
	if err := validRelatedRequest().Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestRelatedRequest_Validate_RequiredScopeFields(t *testing.T) {
	missingProject := validRelatedRequest()
	missingProject.Project = ""
	if err := missingProject.Validate(); err == nil {
		t.Error("expected an error: missing project")
	}

	missingEnv := validRelatedRequest()
	missingEnv.Environment = ""
	if err := missingEnv.Validate(); err == nil {
		t.Error("expected an error: missing environment")
	}

	missingSC := validRelatedRequest()
	missingSC.SecurityContextID = ""
	if err := missingSC.Validate(); err == nil {
		t.Error("expected an error: missing securityContextId")
	}
}

func TestRelatedRequest_Validate_InvalidFact(t *testing.T) {
	r := validRelatedRequest()
	r.Fact.ID = ""
	if err := r.Validate(); err == nil {
		t.Error("expected an error: fact missing its required id")
	}
}

func TestRelatedRequest_Validate_Limit(t *testing.T) {
	within := validRelatedRequest()
	limit := 50
	within.Limit = &limit
	if err := within.Validate(); err != nil {
		t.Errorf("expected valid at the max bound (50, related discovery's own cap), got: %v", err)
	}

	tooHigh := validRelatedRequest()
	over := 51
	tooHigh.Limit = &over
	if err := tooHigh.Validate(); err == nil {
		t.Error("expected an error: limit exceeds related discovery's 50-target cap")
	}

	zero := validRelatedRequest()
	zeroLimit := 0
	zero.Limit = &zeroLimit
	if err := zero.Validate(); err == nil {
		t.Error("expected an error: limit must be positive")
	}
}
