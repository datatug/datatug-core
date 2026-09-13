package apicontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
)

const testFingerprint = "aa02d5bbadc86d4f9ef4d3131f64fe120f0d2e9d7f04b5f75214d4ef60b9c8ba"

func valuePointer(value TypedValue) *TypedValue { return &value }

func validExecutionRef(id string) ExecutionRef {
	return ExecutionRef{StoreID: "evidence", ProjectID: "billing", ExecutionID: id}
}

func validExecutionRecord() ExecutionRecord {
	incident := IncidentRef{StoreID: "incidents", IncidentID: "INC-1"}
	return ExecutionRecord{
		Ref:           validExecutionRef("exec-1"),
		Scope:         ExecutionRecordScope{StoreID: "source-store", Project: "billing", Environment: "production"},
		QueryID:       "invoices/stuck",
		QueryRevision: "e01d44c",
		Parameters: map[string]TypedValueOrSet{
			"CustomerId": ScalarValue(NewIntegerValue("5")),
		},
		BindingsApplied: []Binding{{
			ParameterID: "CustomerId", Value: ScalarValue(NewIntegerValue("5")), FactID: "fact-5",
			Origin: BindingOriginSelection, OriginEvidence: BindingOriginEvidenceClientReported,
		}},
		Principal:         ExecutionPrincipal{ID: "alex", Roles: []string{"admin"}, Groups: []string{"support"}},
		PolicyFingerprint: testFingerprint,
		ExecutedAt:        "2026-09-13T10:00:00Z",
		DurationMS:        42,
		Limitations:       []Limitation{},
		Provenance: Provenance{
			Source: "billing-db", Collection: "Invoice", QueryID: "invoices/stuck",
			Mode: ProvenanceModeLive, ObservedAt: "2026-09-13T10:00:00Z",
			ExecutionProfile: ExecutionProfileProtected,
		},
		AuthorizedFields: []FieldAccessRef{{
			StoreID: "source-store", Project: "billing", Environment: "production", Source: "billing-db",
			Collection: "Invoice", Column: "Total", Entity: "Invoice", Field: "Total",
		}},
		RowCount:          2,
		ResultFingerprint: testFingerprint,
		SnapshotRef:       "snapshot-1",
		Incident:          &incident,
		GrantUses: []GrantRef{{
			Incident: incident, GrantID: "grant-1", ApprovalMutationID: "approve-1",
		}},
		Measurements: []ScalarMeasurement{{
			Projection:   MeasurementProjection{ID: "invoice-total", Column: "Total", Aggregate: MeasurementAggregateSum},
			Completeness: MeasurementComplete, Value: valuePointer(NewDecimalValue("19.95")),
		}},
	}
}

func validExecutionRecordBrief() ExecutionRecordBrief {
	r := validExecutionRecord()
	state := SnapshotState{Availability: SnapshotAvailable, ChangedAt: r.ExecutedAt}
	return ExecutionRecordBrief{
		Ref: r.Ref, Scope: r.Scope, QueryID: r.QueryID, QueryRevision: r.QueryRevision,
		ExecutedAt: r.ExecutedAt, DurationMS: r.DurationMS, RowCount: r.RowCount,
		ResultFingerprint: r.ResultFingerprint, SnapshotRef: r.SnapshotRef, SnapshotState: &state,
		Incident: r.Incident,
	}
}

func TestExecutionRecord_JSONRoundTripAndExactFields(t *testing.T) {
	want := validExecutionRecord()
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got ExecutionRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("round-tripped record should validate: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", got, want)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"ref", "scope", "queryId", "queryRevision", "parameters", "bindingsApplied", "principal",
		"policyFingerprint", "executedAt", "durationMs", "limitations", "provenance", "authorizedFields",
		"rowCount", "resultFingerprint", "snapshotRef", "incident", "grantUses", "measurements",
	} {
		if _, ok := generic[key]; !ok {
			t.Errorf("missing key %q in %s", key, data)
		}
	}
	for _, legacy := range []string{"id", "recordId", "snapshotExpiredAt"} {
		if _, ok := generic[legacy]; ok {
			t.Errorf("legacy key %q present in %s", legacy, data)
		}
	}
	var strict ExecutionRecord
	if err := DecodeStrict(append(data[:len(data)-1], []byte(`,"snapshotExpiredAt":"2026-10-01T00:00:00Z"}`)...), &strict); err == nil {
		t.Fatal("strict decode must reject mutable snapshot expiry on an immutable record")
	}
}

func TestExecutionRecord_ValidateIdentityHashesTimestampsAndCounts(t *testing.T) {
	mutations := map[string]func(*ExecutionRecord){
		"bad ref":                func(r *ExecutionRecord) { r.Ref.ExecutionID = "bad/id" },
		"ref project mismatch":   func(r *ExecutionRecord) { r.Ref.ProjectID = "other" },
		"neither query identity": func(r *ExecutionRecord) { r.QueryID, r.QueryRevision = "", "" },
		"both query identities":  func(r *ExecutionRecord) { r.DTQLHash = testFingerprint },
		"revision without query": func(r *ExecutionRecord) { r.QueryID, r.DTQLHash = "", testFingerprint },
		"bad policy hash":        func(r *ExecutionRecord) { r.PolicyFingerprint = "ABC" },
		"bad result hash":        func(r *ExecutionRecord) { r.ResultFingerprint = "nope" },
		"bad executedAt":         func(r *ExecutionRecord) { r.ExecutedAt = "yesterday" },
		"negative duration":      func(r *ExecutionRecord) { r.DurationMS = -1 },
		"negative rows":          func(r *ExecutionRecord) { r.RowCount = -1 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			r := validExecutionRecord()
			mutate(&r)
			if err := r.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	adhoc := validExecutionRecord()
	adhoc.QueryID, adhoc.QueryRevision, adhoc.DTQLHash, adhoc.Provenance.QueryID = "", "", testFingerprint, ""
	if err := adhoc.Validate(); err != nil {
		t.Fatalf("valid ad-hoc record rejected: %v", err)
	}
}

func TestExecutionRecord_GrantUsesAndFieldManifest(t *testing.T) {
	if err := validExecutionRecord().Validate(); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
	for name, mutate := range map[string]func(*ExecutionRecord){
		"grant without incident": func(r *ExecutionRecord) { r.Incident = nil },
		"cross incident":         func(r *ExecutionRecord) { r.GrantUses[0].Incident.IncidentID = "INC-2" },
		"bad grant id":           func(r *ExecutionRecord) { r.GrantUses[0].GrantID = "bad/id" },
		"bad approval mutation":  func(r *ExecutionRecord) { r.GrantUses[0].ApprovalMutationID = "../bad" },
		"empty present grants":   func(r *ExecutionRecord) { r.GrantUses = []GrantRef{} },
		"bad field store":        func(r *ExecutionRecord) { r.AuthorizedFields[0].StoreID = "" },
		"half semantic field":    func(r *ExecutionRecord) { r.AuthorizedFields[0].Field = "" },
		"duplicate field":        func(r *ExecutionRecord) { r.AuthorizedFields = append(r.AuthorizedFields, r.AuthorizedFields[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			r := validExecutionRecord()
			mutate(&r)
			if err := r.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	ordinary := validExecutionRecord()
	ordinary.Incident, ordinary.GrantUses = nil, nil
	if err := ordinary.Validate(); err != nil {
		t.Fatalf("ordinary policy record rejected: %v", err)
	}
}

func TestExecutionRecord_ValidatesEveryNestedReceiptSurface(t *testing.T) {
	for name, mutate := range map[string]func(*ExecutionRecord){
		"scope store":               func(r *ExecutionRecord) { r.Scope.StoreID = "bad/store" },
		"scope environment":         func(r *ExecutionRecord) { r.Scope.Environment = " " },
		"parameter id":              func(r *ExecutionRecord) { r.Parameters[" "] = ScalarValue(NewStringValue("x")) },
		"parameter value":           func(r *ExecutionRecord) { r.Parameters["CustomerId"] = ScalarValue(NewIntegerValue("01")) },
		"binding":                   func(r *ExecutionRecord) { r.BindingsApplied[0].Value = ScalarValue(NewIntegerValue("01")) },
		"duplicate binding":         func(r *ExecutionRecord) { r.BindingsApplied = append(r.BindingsApplied, r.BindingsApplied[0]) },
		"principal":                 func(r *ExecutionRecord) { r.Principal.ID = "" },
		"principal role":            func(r *ExecutionRecord) { r.Principal.Roles = []string{""} },
		"principal group":           func(r *ExecutionRecord) { r.Principal.Groups = []string{" "} },
		"limitation":                func(r *ExecutionRecord) { r.Limitations = []Limitation{{}} },
		"provenance":                func(r *ExecutionRecord) { r.Provenance.Source = "" },
		"provenance query mismatch": func(r *ExecutionRecord) { r.Provenance.QueryID = "other" },
		"field source mismatch":     func(r *ExecutionRecord) { r.AuthorizedFields[0].Source = "other" },
		"snapshot ref":              func(r *ExecutionRecord) { r.SnapshotRef = " snapshot " },
		"incident":                  func(r *ExecutionRecord) { r.Incident.IncidentID = "bad/id" },
		"duplicate grant":           func(r *ExecutionRecord) { r.GrantUses = append(r.GrantUses, r.GrantUses[0]) },
		"measurement":               func(r *ExecutionRecord) { r.Measurements[0].Value = valuePointer(NewStringValue("bad")) },
		"duplicate measurement":     func(r *ExecutionRecord) { r.Measurements = append(r.Measurements, r.Measurements[0]) },
		"policy-limited aggregate": func(r *ExecutionRecord) {
			r.Limitations = []Limitation{{Policy: "restricted", HiddenColumns: []string{}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			record := validExecutionRecord()
			mutate(&record)
			if err := record.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	rowCount := validExecutionRecord()
	rowCount.Limitations = []Limitation{{Policy: "restricted", HiddenColumns: []string{}}}
	rowCount.Measurements[0] = ScalarMeasurement{
		Projection:   MeasurementProjection{ID: "row-count", Aggregate: MeasurementAggregateRowCount},
		Completeness: MeasurementComplete, Value: valuePointer(NewIntegerValue("2")),
	}
	if err := rowCount.Validate(); err != nil {
		t.Fatalf("policy-limited rowCount should remain complete: %v", err)
	}
}

func TestScalarMeasurement_ClosedUnionAndNumericValues(t *testing.T) {
	projection := MeasurementProjection{ID: "total", Column: "Total", Aggregate: MeasurementAggregateSum}
	for _, value := range []TypedValue{NewNumberValue(1.5), NewIntegerValue("2"), NewDecimalValue("3.25")} {
		measurement := ScalarMeasurement{Projection: projection, Completeness: MeasurementComplete, Value: valuePointer(value)}
		if err := measurement.Validate(); err != nil {
			t.Errorf("numeric %s rejected: %v", value.Type, err)
		}
	}
	for _, value := range []TypedValue{NewStringValue("1"), NewBooleanValue(true), NewDateValue("2026-09-13"), NewNullValue()} {
		measurement := ScalarMeasurement{Projection: projection, Completeness: MeasurementComplete, Value: valuePointer(value)}
		if err := measurement.Validate(); err == nil {
			t.Errorf("non-numeric %s accepted", value.Type)
		}
	}
	for name, measurement := range map[string]ScalarMeasurement{
		"complete missing value": {Projection: projection, Completeness: MeasurementComplete},
		"complete with reason":   {Projection: projection, Completeness: MeasurementComplete, Value: valuePointer(NewIntegerValue("1")), Reason: MeasurementReasonNoRows},
		"unavailable with value": {Projection: projection, Completeness: MeasurementUnavailable, Value: valuePointer(NewIntegerValue("1")), Reason: MeasurementReasonNoRows},
		"unavailable bad reason": {Projection: projection, Completeness: MeasurementUnavailable, Reason: "unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := measurement.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	for _, reason := range []string{MeasurementReasonNoRows, MeasurementReasonNonNumeric, MeasurementReasonPolicyLimited, MeasurementReasonTruncated, MeasurementReasonSourceRefused} {
		if err := (ScalarMeasurement{Projection: projection, Completeness: MeasurementUnavailable, Reason: reason}).Validate(); err != nil {
			t.Errorf("reason %q rejected: %v", reason, err)
		}
	}
}

func TestFingerprintRecordset_DeterministicOrderSensitiveAndNormalized(t *testing.T) {
	recordset := Recordset{
		Columns: []Column{{Name: "id", Type: "integer"}, {Name: "name", Type: "string"}},
		Rows:    [][]TypedValue{{NewIntegerValue("1"), NewStringValue("one")}, {NewIntegerValue("2"), NewStringValue("two")}},
	}
	first, err := FingerprintRecordset(recordset)
	if err != nil {
		t.Fatal(err)
	}
	second, err := FingerprintRecordset(recordset)
	if err != nil || first != second {
		t.Fatalf("identical recordsets differ: %s != %s (%v)", first, second, err)
	}
	canonical := `{"columns":[{"name":"id","type":"integer"},{"name":"name","type":"string"}],"rows":[[{"type":"integer","value":"1"},{"type":"string","value":"one"}],[{"type":"integer","value":"2"},{"type":"string","value":"two"}]]}`
	wantDigest := sha256.Sum256([]byte(canonical))
	if want := hex.EncodeToString(wantDigest[:]); first != want {
		t.Fatalf("fingerprint = %s, want %s", first, want)
	}
	reordered := recordset
	reordered.Rows = [][]TypedValue{recordset.Rows[1], recordset.Rows[0]}
	different, err := FingerprintRecordset(reordered)
	if err != nil || different == first {
		t.Fatalf("row-order change did not change fingerprint: %s (%v)", different, err)
	}
	nilFingerprint, err := FingerprintRecordset(Recordset{})
	if err != nil {
		t.Fatal(err)
	}
	emptyFingerprint, err := FingerprintRecordset(Recordset{Columns: []Column{}, Rows: [][]TypedValue{}})
	if err != nil || nilFingerprint != emptyFingerprint {
		t.Fatalf("nil and empty recordsets differ: %s != %s (%v)", nilFingerprint, emptyFingerprint, err)
	}
	nilRow, err := FingerprintRecordset(Recordset{Columns: []Column{}, Rows: [][]TypedValue{nil}})
	emptyRow, err2 := FingerprintRecordset(Recordset{Columns: []Column{}, Rows: [][]TypedValue{{}}})
	if err != nil || err2 != nil || nilRow != emptyRow {
		t.Fatalf("nil and empty zero-column rows differ: %s != %s (%v, %v)", nilRow, emptyRow, err, err2)
	}
	invalid := Recordset{Columns: []Column{{Name: "id", Type: "integer"}}, Rows: [][]TypedValue{{}}}
	if fingerprint, err := FingerprintRecordset(invalid); err == nil || fingerprint != "" {
		t.Fatalf("invalid fingerprint = %q, err = %v", fingerprint, err)
	}
}

func TestExecutionListAndSnapshotStateContracts(t *testing.T) {
	limit := 100
	incident := IncidentRef{StoreID: "incidents", IncidentID: "INC-1"}
	request := ExecutionListRequest{
		Scope:   Scope{StoreID: "evidence", Project: "billing", Environment: "production", SecurityContextID: "sc-1"},
		QueryID: "invoices/stuck", Incident: &incident, Since: "2026-09-13T09:00:00Z",
		Until: "2026-09-13T11:00:00Z", Limit: &limit,
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid list request rejected: %v", err)
	}
	for name, mutate := range map[string]func(*ExecutionListRequest){
		"scope":    func(r *ExecutionListRequest) { r.Project = "" },
		"query":    func(r *ExecutionListRequest) { r.QueryID = " invoices/stuck " },
		"incident": func(r *ExecutionListRequest) { r.Incident.IncidentID = "bad/id" },
		"since":    func(r *ExecutionListRequest) { r.Since = "bad" },
		"until":    func(r *ExecutionListRequest) { r.Until = "bad" },
		"range":    func(r *ExecutionListRequest) { r.Until = "2026-09-13T08:00:00Z" },
		"limit":    func(r *ExecutionListRequest) { zero := 0; r.Limit = &zero },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := request
			incidentCopy := *request.Incident
			candidate.Incident = &incidentCopy
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	response := ExecutionListResponse{Executions: []ExecutionRecordBrief{validExecutionRecordBrief()}}
	if err := response.Validate(); err != nil {
		t.Fatalf("valid list response rejected: %v", err)
	}
	response.Executions[0].Ref.ExecutionID = ""
	if err := response.Validate(); err == nil {
		t.Fatal("invalid brief should be rejected")
	}
}

func TestExecutionRecordBrief_ValidatesIdentityAndSnapshotLifecycle(t *testing.T) {
	for name, mutate := range map[string]func(*ExecutionRecordBrief){
		"scope":                  func(b *ExecutionRecordBrief) { b.Scope.Project = "" },
		"project mismatch":       func(b *ExecutionRecordBrief) { b.Ref.ProjectID = "other" },
		"query identity":         func(b *ExecutionRecordBrief) { b.QueryID = "" },
		"timestamp":              func(b *ExecutionRecordBrief) { b.ExecutedAt = "bad" },
		"duration":               func(b *ExecutionRecordBrief) { b.DurationMS = -1 },
		"fingerprint":            func(b *ExecutionRecordBrief) { b.ResultFingerprint = "bad" },
		"state without ref":      func(b *ExecutionRecordBrief) { b.SnapshotRef = "" },
		"ref without state":      func(b *ExecutionRecordBrief) { b.SnapshotState = nil },
		"snapshot ref":           func(b *ExecutionRecordBrief) { b.SnapshotRef = " snapshot " },
		"snapshot state":         func(b *ExecutionRecordBrief) { b.SnapshotState.Availability = "bad" },
		"state before execution": func(b *ExecutionRecordBrief) { b.SnapshotState.ChangedAt = "2026-09-13T09:00:00Z" },
		"terminal at execution":  func(b *ExecutionRecordBrief) { b.SnapshotState.Availability = SnapshotExpired },
		"incident":               func(b *ExecutionRecordBrief) { b.Incident.IncidentID = "bad/id" },
	} {
		t.Run(name, func(t *testing.T) {
			brief := validExecutionRecordBrief()
			state := *brief.SnapshotState
			incident := *brief.Incident
			brief.SnapshotState, brief.Incident = &state, &incident
			mutate(&brief)
			if err := brief.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	withoutSnapshot := validExecutionRecordBrief()
	withoutSnapshot.SnapshotRef, withoutSnapshot.SnapshotState = "", nil
	if err := withoutSnapshot.Validate(); err != nil {
		t.Fatalf("brief without captured snapshot rejected: %v", err)
	}
}

func TestSnapshotStateAndReadResponse(t *testing.T) {
	available := SnapshotState{Availability: SnapshotAvailable, ChangedAt: "2026-09-13T10:00:00Z"}
	for _, state := range []SnapshotState{
		available,
		{Availability: SnapshotExpired, ChangedAt: "2026-10-13T10:00:00Z", Reason: "retention"},
		{Availability: SnapshotDeleted, ChangedAt: "2026-10-14T10:00:00Z", Reason: "operator-request"},
	} {
		if err := state.Validate(); err != nil {
			t.Errorf("valid snapshot state rejected: %v", err)
		}
	}
	for name, state := range map[string]SnapshotState{
		"unknown":          {Availability: "missing", ChangedAt: "2026-09-13T10:00:00Z"},
		"bad time":         {Availability: SnapshotExpired, ChangedAt: "bad"},
		"available reason": {Availability: SnapshotAvailable, ChangedAt: "2026-09-13T10:00:00Z", Reason: "why"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := state.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	recordset := validResult().Recordset
	response := SnapshotReadResponse{
		Execution: validExecutionRef("exec-1"), SnapshotRef: "snapshot-1", SnapshotState: available,
		Recordset: &recordset, Limitations: []Limitation{},
	}
	if err := response.Validate(); err != nil {
		t.Fatalf("available snapshot rejected: %v", err)
	}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"provenance", "bindingsApplied", "recordId", "snapshotExpiredAt"} {
		if _, ok := fields[forbidden]; ok {
			t.Errorf("snapshot evidence contains forbidden %q: %s", forbidden, data)
		}
	}
	expired := SnapshotReadResponse{
		Execution: validExecutionRef("exec-1"), SnapshotRef: "snapshot-1",
		SnapshotState: SnapshotState{Availability: SnapshotExpired, ChangedAt: "2026-10-13T10:00:00Z", Reason: "retention"},
	}
	if err := expired.Validate(); err != nil {
		t.Fatalf("explicit expired snapshot rejected: %v", err)
	}
	for name, mutate := range map[string]func(*SnapshotReadResponse){
		"available without rows": func(r *SnapshotReadResponse) { r.Recordset = nil },
		"bad execution":          func(r *SnapshotReadResponse) { r.Execution.ExecutionID = "" },
		"bad snapshot ref":       func(r *SnapshotReadResponse) { r.SnapshotRef = " " },
		"expired with rows":      func(r *SnapshotReadResponse) { r.SnapshotState = expired.SnapshotState },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := response
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestSnapshotState_ValidateTransition(t *testing.T) {
	available := SnapshotState{Availability: SnapshotAvailable, ChangedAt: "2026-09-13T10:00:00Z"}
	expired := SnapshotState{Availability: SnapshotExpired, ChangedAt: "2026-10-13T10:00:00Z", Reason: "retention"}
	deleted := SnapshotState{Availability: SnapshotDeleted, ChangedAt: "2026-10-14T10:00:00Z", Reason: "operator-request"}
	for _, next := range []SnapshotState{available, expired, deleted} {
		if err := available.ValidateTransition(next); err != nil {
			t.Errorf("valid transition to %q rejected: %v", next.Availability, err)
		}
	}
	if err := expired.ValidateTransition(expired); err != nil {
		t.Fatalf("idempotent terminal retry rejected: %v", err)
	}
	for name, testCase := range map[string]struct {
		current SnapshotState
		next    SnapshotState
	}{
		"terminal to terminal":  {expired, deleted},
		"terminal to available": {expired, available},
		"same-time advance":     {available, SnapshotState{Availability: SnapshotExpired, ChangedAt: available.ChangedAt}},
		"invalid current":       {SnapshotState{Availability: "bad", ChangedAt: available.ChangedAt}, expired},
		"invalid next":          {available, SnapshotState{Availability: SnapshotExpired, ChangedAt: "bad"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := testCase.current.ValidateTransition(testCase.next); err == nil {
				t.Fatal("expected invalid lifecycle transition")
			}
		})
	}
}

func validSeriesRequest() ExecutionSeriesRequest {
	return ExecutionSeriesRequest{
		Scope: Scope{StoreID: "evidence", Project: "billing", Environment: "production", SecurityContextID: "sc-1"},
		Partition: ExecutionSeriesPartition{
			EvidenceStoreID:   "evidence",
			SourceScope:       ExecutionRecordScope{StoreID: "source-store", Project: "billing", Environment: "production"},
			Source:            "billing-db",
			PolicyFingerprint: testFingerprint,
			QueryID:           "invoices/stuck", QueryRevision: "e01d44c", BindingsApplied: []Binding{},
			Projection: MeasurementProjection{ID: "row-count", Aggregate: MeasurementAggregateRowCount},
		},
	}
}

func TestExecutionSeriesCompletePartitionAndPointUnion(t *testing.T) {
	request := validSeriesRequest()
	if err := request.Validate(); err != nil {
		t.Fatalf("valid series request rejected: %v", err)
	}
	for name, mutate := range map[string]func(*ExecutionSeriesRequest){
		"evidence store": func(r *ExecutionSeriesRequest) { r.Partition.EvidenceStoreID = "other" },
		"source store":   func(r *ExecutionSeriesRequest) { r.Partition.SourceScope.StoreID = "" },
		"source":         func(r *ExecutionSeriesRequest) { r.Partition.Source = "" },
		"project":        func(r *ExecutionSeriesRequest) { r.Partition.SourceScope.Project = "other" },
		"environment":    func(r *ExecutionSeriesRequest) { r.Partition.SourceScope.Environment = "other" },
		"policy":         func(r *ExecutionSeriesRequest) { r.Partition.PolicyFingerprint = "bad" },
		"query":          func(r *ExecutionSeriesRequest) { r.Partition.QueryID = "" },
		"query revision": func(r *ExecutionSeriesRequest) { r.Partition.QueryRevision = "" },
		"projection":     func(r *ExecutionSeriesRequest) { r.Partition.Projection.ID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := request
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	first := NewIntegerValue("37")
	response := ExecutionSeriesResponse{Points: []ExecutionSeriesPoint{
		{ExecutedAt: "2026-09-13T10:00:00Z", Execution: validExecutionRef("exec-1"), Completeness: MeasurementComplete, Value: &first},
		{ExecutedAt: "2026-09-13T11:00:00Z", Execution: validExecutionRef("exec-2"), Completeness: MeasurementUnavailable, Reason: MeasurementReasonTruncated},
	}, Omitted: true}
	if err := response.Validate(); err != nil {
		t.Fatalf("valid series response rejected: %v", err)
	}
	for name, mutate := range map[string]func(*ExecutionSeriesResponse){
		"point timestamp":            func(r *ExecutionSeriesResponse) { r.Points[0].ExecutedAt = "bad" },
		"point execution":            func(r *ExecutionSeriesResponse) { r.Points[0].Execution.ExecutionID = "" },
		"point value":                func(r *ExecutionSeriesResponse) { r.Points[0].Value = valuePointer(NewStringValue("37")) },
		"unavailable missing reason": func(r *ExecutionSeriesResponse) { r.Points[1].Reason = "" },
		"order":                      func(r *ExecutionSeriesResponse) { r.Points[0], r.Points[1] = r.Points[1], r.Points[0] },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := response
			candidate.Points = append([]ExecutionSeriesPoint(nil), response.Points...)
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
