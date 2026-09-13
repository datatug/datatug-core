package apicontract

import (
	"encoding/json"
	"testing"
)

func validExecutionRequestSaved() ExecutionRequest {
	return ExecutionRequest{
		Project:           "demo-project-1",
		Environment:       "local",
		SecurityContextID: "sc1",
		QueryID:           "customers/customer-purchases-by-genre",
		Parameters:        map[string]TypedValueOrSet{"CustomerId": ScalarValue(NewIntegerValue("5"))},
		BindingOrigins:    []BindingOriginEntry{{ParameterID: "CustomerId", Origin: "selection", FactID: "f1"}},
		Mode:              "live",
	}
}

func TestExecutionRequest_JSONFieldNames(t *testing.T) {
	r := validExecutionRequestSaved()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"project", "environment", "securityContextId", "queryId", "parameters", "bindingOrigins", "mode"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("missing key %q in %s", key, data)
		}
	}
	for _, key := range []string{"storeId", "source", "dtql", "snapshotId", "limit", "incident", "record", "snapshot", "measurementProjections"} {
		if _, ok := generic[key]; ok {
			t.Errorf("expected %q to be omitted when absent, got %s", key, data)
		}
	}
}

func TestExecutionRequest_RecordAndSnapshot(t *testing.T) {
	recorded := validExecutionRequestSaved()
	recorded.Record = true
	recorded.Snapshot = true
	if err := recorded.Validate(); err != nil {
		t.Fatalf("recorded snapshot should be valid: %v", err)
	}
	data, err := json.Marshal(recorded)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"record", "snapshot"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("missing key %q in %s", key, data)
		}
	}

	withoutRecord := validExecutionRequestSaved()
	withoutRecord.Snapshot = true
	if err := withoutRecord.Validate(); err == nil {
		t.Fatal("snapshot without record should be rejected")
	}
}

func TestExecutionRequest_MeasurementProjectionsRequireRecordingAndUniqueIDs(t *testing.T) {
	request := validExecutionRequestSaved()
	request.Record = true
	request.MeasurementProjections = []MeasurementProjection{
		{ID: "row-count", Aggregate: MeasurementAggregateRowCount},
		{ID: "total", Column: "Total", Aggregate: MeasurementAggregateSum},
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid measurement projections rejected: %v", err)
	}
	withoutRecord := request
	withoutRecord.Record = false
	if err := withoutRecord.Validate(); err == nil {
		t.Fatal("measurement projections without recording should be rejected")
	}
	duplicate := request
	duplicate.MeasurementProjections = append([]MeasurementProjection(nil), request.MeasurementProjections...)
	duplicate.MeasurementProjections[1].ID = "row-count"
	if err := duplicate.Validate(); err == nil {
		t.Fatal("duplicate projection ids should be rejected")
	}
	badShape := request
	badShape.MeasurementProjections = []MeasurementProjection{{ID: "bad", Column: "Total", Aggregate: MeasurementAggregateRowCount}}
	if err := badShape.Validate(); err == nil {
		t.Fatal("rowCount with a column should be rejected")
	}
}

func TestExecutionRequest_SetBindingAndAdHocRejection(t *testing.T) {
	set := mustTypedValueSet(t, NewIntegerValue("1"), NewIntegerValue("2"))
	request := validExecutionRequestSaved()
	request.Parameters["CustomerId"] = SetValue(set)
	request.BindingOrigins[0].FactID = ""
	request.BindingOrigins[0].ValueFactIDs = [][]string{{"fact-1"}, {"fact-2"}}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid saved-query set rejected: %v", err)
	}
	misaligned := request
	misaligned.BindingOrigins = append([]BindingOriginEntry(nil), request.BindingOrigins...)
	misaligned.BindingOrigins[0].ValueFactIDs = [][]string{{"fact-1"}}
	if err := misaligned.Validate(); err == nil {
		t.Fatal("misaligned set provenance should be rejected")
	}
	adhoc := request
	adhoc.QueryID = ""
	adhoc.DTQL = "from: Customer"
	adhoc.Source = "source-1"
	if err := adhoc.Validate(); err == nil {
		t.Fatal("ad-hoc DTQL must reject set values")
	}
}

func TestExecutionRequest_Validate_Saved(t *testing.T) {
	if err := validExecutionRequestSaved().Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestExecutionRequest_Validate_AdHocDTQLRequiresSource(t *testing.T) {
	r := ExecutionRequest{
		Project: "p1", Environment: "local", SecurityContextID: "sc1",
		DTQL: "from:\n  name: Customer\n",
		Mode: "live",
	}
	if err := r.Validate(); err == nil {
		t.Error("expected an error: ad-hoc DTQL requires source")
	}
	r.Source = "chinook"
	if err := r.Validate(); err != nil {
		t.Errorf("expected valid once source is set, got: %v", err)
	}
}

func TestExecutionRequest_Validate_ExactlyOneOfQueryIDOrDTQL(t *testing.T) {
	neither := ExecutionRequest{Project: "p1", Environment: "local", SecurityContextID: "sc1", Mode: "live"}
	if err := neither.Validate(); err == nil {
		t.Error("expected an error: neither queryId nor dtql set")
	}

	both := validExecutionRequestSaved()
	both.DTQL = "from:\n  name: Customer\n"
	both.Source = "chinook"
	if err := both.Validate(); err == nil {
		t.Error("expected an error: both queryId and dtql set")
	}
}

func TestExecutionRequest_Validate_RequiredScopeFields(t *testing.T) {
	r := validExecutionRequestSaved()
	r.Project = ""
	if err := r.Validate(); err == nil {
		t.Error("expected an error: missing project")
	}
}

func TestExecutionRequest_Validate_UnknownParameterValue(t *testing.T) {
	r := validExecutionRequestSaved()
	r.Parameters["CustomerId"] = ScalarValue(NewIntegerValue("not-canonical"))
	if err := r.Validate(); err == nil {
		t.Error("expected an error: invalid parameter value")
	}
}

func TestExecutionRequest_Validate_BindingOriginsMustMatchParameterKeysExactly(t *testing.T) {
	// "bindingOrigins... is checked for exactly the submitted keys"
	extra := validExecutionRequestSaved()
	extra.BindingOrigins = append(extra.BindingOrigins, BindingOriginEntry{ParameterID: "Extra", Origin: "manual"})
	if err := extra.Validate(); err == nil {
		t.Error("expected an error: bindingOrigins names a parameter not in parameters")
	}

	missing := validExecutionRequestSaved()
	missing.Parameters["Extra"] = ScalarValue(NewStringValue("x"))
	if err := missing.Validate(); err == nil {
		t.Error("expected an error: parameters has a key with no matching bindingOrigins entry")
	}
}

func TestExecutionRequest_Validate_BindingOriginsOrigin(t *testing.T) {
	r := validExecutionRequestSaved()
	r.BindingOrigins[0].Origin = "bogus"
	if err := r.Validate(); err == nil {
		t.Error("expected an error: invalid bindingOrigins origin")
	}
}

func TestExecutionRequest_Validate_DuplicateBindingOriginsEntry(t *testing.T) {
	r := validExecutionRequestSaved()
	r.BindingOrigins = append(r.BindingOrigins, BindingOriginEntry{ParameterID: "CustomerId", Origin: "manual"})
	if err := r.Validate(); err == nil {
		t.Error("expected an error: duplicate bindingOrigins entry for the same parameter")
	}
}

func TestExecutionRequest_Validate_MissingEnvironmentOrSecurityContext(t *testing.T) {
	missingEnv := validExecutionRequestSaved()
	missingEnv.Environment = ""
	if err := missingEnv.Validate(); err == nil {
		t.Error("expected an error: missing environment")
	}

	missingSC := validExecutionRequestSaved()
	missingSC.SecurityContextID = ""
	if err := missingSC.Validate(); err == nil {
		t.Error("expected an error: missing securityContextId")
	}
}

func TestBindingOriginEntry_Validate(t *testing.T) {
	if err := (BindingOriginEntry{ParameterID: "p", Origin: "manual"}).Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
	if err := (BindingOriginEntry{Origin: "manual"}).Validate(); err == nil {
		t.Error("expected an error: missing parameterId")
	}
	if err := (BindingOriginEntry{ParameterID: "p", Origin: "bogus"}).Validate(); err == nil {
		t.Error("expected an error: invalid origin")
	}
}

func TestExecutionRequest_Validate_Mode(t *testing.T) {
	r := validExecutionRequestSaved()
	r.Mode = "cached"
	if err := r.Validate(); err == nil {
		t.Error("expected an error: invalid mode")
	}

	snapshotNoID := validExecutionRequestSaved()
	snapshotNoID.Mode = "snapshot"
	if err := snapshotNoID.Validate(); err == nil {
		t.Error("expected an error: snapshot mode requires snapshotId")
	}

	snapshotWithID := validExecutionRequestSaved()
	snapshotWithID.Mode = "snapshot"
	snapshotWithID.SnapshotID = "snap-1"
	if err := snapshotWithID.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestExecutionRequest_Validate_Limit(t *testing.T) {
	within := validExecutionRequestSaved()
	limit := 500
	within.Limit = &limit
	if err := within.Validate(); err != nil {
		t.Errorf("expected valid at the max bound, got: %v", err)
	}

	tooHigh := validExecutionRequestSaved()
	over := 501
	tooHigh.Limit = &over
	if err := tooHigh.Validate(); err == nil {
		t.Error("expected an error: limit exceeds the maximum of 500")
	}

	zero := validExecutionRequestSaved()
	zeroLimit := 0
	zero.Limit = &zeroLimit
	if err := zero.Validate(); err == nil {
		t.Error("expected an error: limit must be positive")
	}
}

func TestExecutionRequest_IncidentRef(t *testing.T) {
	request := validExecutionRequestSaved()
	request.StoreID = "ops"
	request.Incident = &IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	if err := request.Validate(); err != nil {
		t.Fatalf("qualified incident request should be valid: %v", err)
	}
	request.Incident.IncidentID = "bad/id"
	if err := request.Validate(); err == nil {
		t.Fatal("invalid incident ref should be rejected")
	}
}

func TestExecutionRequest_RejectsInvalidQualifiedScope(t *testing.T) {
	request := validExecutionRequestSaved()
	request.StoreID = "bad/store"
	if err := request.Validate(); err == nil {
		t.Fatal("invalid qualified store scope should be rejected")
	}
}
