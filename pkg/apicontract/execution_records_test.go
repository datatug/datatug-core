package apicontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
)

const testFingerprint = "aa02d5bbadc86d4f9ef4d3131f64fe120f0d2e9d7f04b5f75214d4ef60b9c8ba"

func validExecutionRecord() ExecutionRecord {
	return ExecutionRecord{
		ID:            "exec-1",
		Scope:         ExecutionRecordScope{Project: "billing", Environment: "production"},
		QueryID:       "invoices/stuck",
		QueryRevision: "a837758",
		Parameters: map[string]TypedValue{
			"CustomerId": NewIntegerValue("5"),
		},
		BindingsApplied: []Binding{{
			ParameterID: "CustomerId", Value: NewIntegerValue("5"),
			Origin: BindingOriginSelection, OriginEvidence: BindingOriginEvidenceClientReported,
		}},
		Principal:   ExecutionPrincipal{ID: "alex", Roles: []string{"admin"}, Groups: []string{"support"}},
		ExecutedAt:  "2026-09-13T10:00:00Z",
		DurationMS:  42,
		Limitations: []Limitation{},
		Provenance: Provenance{
			Source: "billing-db", Collection: "Invoice", QueryID: "invoices/stuck",
			Mode: ProvenanceModeLive, ObservedAt: "2026-09-13T10:00:00Z",
			ExecutionProfile: ExecutionProfileProtected,
			Incident:         &IncidentRef{StoreID: "ops", IncidentID: "INC-1"},
		},
		RowCount:          2,
		ResultFingerprint: testFingerprint,
		SnapshotRef:       "snapshot-1",
	}
}

func validExecutionRecordBrief() ExecutionRecordBrief {
	r := validExecutionRecord()
	return ExecutionRecordBrief{
		ID: r.ID, Scope: r.Scope, QueryID: r.QueryID, QueryRevision: r.QueryRevision,
		ExecutedAt: r.ExecutedAt, DurationMS: r.DurationMS, RowCount: r.RowCount,
		ResultFingerprint: r.ResultFingerprint, SnapshotRef: r.SnapshotRef, Incident: r.Provenance.Incident,
	}
}

func TestExecutionRecord_JSONRoundTrip(t *testing.T) {
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
		"id", "scope", "queryId", "queryRevision", "parameters", "bindingsApplied", "principal",
		"executedAt", "durationMs", "limitations", "provenance", "rowCount", "resultFingerprint", "snapshotRef",
	} {
		if _, ok := generic[key]; !ok {
			t.Errorf("missing key %q in %s", key, data)
		}
	}
	for _, key := range []string{"dtqlHash", "snapshotExpiredAt"} {
		if _, ok := generic[key]; ok {
			t.Errorf("expected %q omitted from %s", key, data)
		}
	}
}

func TestExecutionRecord_ValidateQueryIdentity(t *testing.T) {
	neither := validExecutionRecord()
	neither.QueryID = ""
	neither.QueryRevision = ""
	if err := neither.Validate(); err == nil {
		t.Fatal("neither queryId nor dtqlHash should be rejected")
	}

	both := validExecutionRecord()
	both.DTQLHash = testFingerprint
	if err := both.Validate(); err == nil {
		t.Fatal("both queryId and dtqlHash should be rejected")
	}

	adhoc := validExecutionRecord()
	adhoc.QueryID = ""
	adhoc.QueryRevision = ""
	adhoc.DTQLHash = testFingerprint
	adhoc.Provenance.QueryID = ""
	if err := adhoc.Validate(); err != nil {
		t.Fatalf("dtqlHash-only record should be valid: %v", err)
	}

	adhoc.QueryRevision = "a837758"
	if err := adhoc.Validate(); err == nil {
		t.Fatal("queryRevision without queryId should be rejected")
	}

	badQuery := validExecutionRecord()
	badQuery.QueryID = " invoices/stuck "
	if err := badQuery.Validate(); err == nil {
		t.Fatal("non-canonical queryId should be rejected")
	}

	badRevision := validExecutionRecord()
	badRevision.QueryRevision = " a837758 "
	if err := badRevision.Validate(); err == nil {
		t.Fatal("non-canonical queryRevision should be rejected")
	}
}

func TestExecutionRecord_ValidateHashesTimestampsAndCounts(t *testing.T) {
	mutations := map[string]func(*ExecutionRecord){
		"bad id":            func(r *ExecutionRecord) { r.ID = "bad/id" },
		"bad dtql hash":     func(r *ExecutionRecord) { r.QueryID = ""; r.QueryRevision = ""; r.DTQLHash = "nope" },
		"bad fingerprint":   func(r *ExecutionRecord) { r.ResultFingerprint = "ABC" },
		"bad executedAt":    func(r *ExecutionRecord) { r.ExecutedAt = "yesterday" },
		"negative duration": func(r *ExecutionRecord) { r.DurationMS = -1 },
		"negative rows":     func(r *ExecutionRecord) { r.RowCount = -1 },
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

	uppercaseHash := validExecutionRecord()
	uppercaseHash.ResultFingerprint = "AA02D5BBADC86D4F9EF4D3131F64FE120F0D2E9D7F04B5F75214D4EF60B9C8BA"
	if err := uppercaseHash.Validate(); err == nil {
		t.Fatal("uppercase fingerprint should be rejected")
	}
}

func TestExecutionRecord_ValidateSnapshotState(t *testing.T) {
	active := validExecutionRecord()
	if err := active.Validate(); err != nil {
		t.Fatalf("active snapshot should be valid: %v", err)
	}

	none := validExecutionRecord()
	none.SnapshotRef = ""
	if err := none.Validate(); err != nil {
		t.Fatalf("record without stored snapshot should be valid: %v", err)
	}

	expired := none
	expired.SnapshotExpiredAt = "2026-10-13T10:00:00Z"
	if err := expired.Validate(); err != nil {
		t.Fatalf("expired snapshot state should be valid: %v", err)
	}

	both := expired
	both.SnapshotRef = "snapshot-1"
	if err := both.Validate(); err == nil {
		t.Fatal("active and expired snapshot state together should be rejected")
	}

	badRef := active
	badRef.SnapshotRef = " blob://bucket/snapshot-1 "
	if err := badRef.Validate(); err == nil {
		t.Fatal("non-canonical snapshotRef should be rejected")
	}
	backendOpaqueRef := active
	backendOpaqueRef.SnapshotRef = "blob://bucket/snapshot-1"
	if err := backendOpaqueRef.Validate(); err != nil {
		t.Fatalf("snapshotRef must remain backend-opaque: %v", err)
	}

	badTime := none
	badTime.SnapshotExpiredAt = "not-a-time"
	if err := badTime.Validate(); err == nil {
		t.Fatal("invalid snapshotExpiredAt should be rejected")
	}

	notAfter := none
	notAfter.SnapshotExpiredAt = notAfter.ExecutedAt
	if err := notAfter.Validate(); err == nil {
		t.Fatal("snapshotExpiredAt at executedAt should be rejected")
	}
}

func TestExecutionRecord_ValidatesNestedValues(t *testing.T) {
	mutations := map[string]func(*ExecutionRecord){
		"scope":        func(r *ExecutionRecord) { r.Scope.Project = "" },
		"parameter id": func(r *ExecutionRecord) { r.Parameters[" "] = NewStringValue("x") },
		"parameter":    func(r *ExecutionRecord) { r.Parameters["CustomerId"] = NewIntegerValue("bad") },
		"binding":      func(r *ExecutionRecord) { r.BindingsApplied[0].Value = NewIntegerValue("bad") },
		"principal":    func(r *ExecutionRecord) { r.Principal.ID = "" },
		"role":         func(r *ExecutionRecord) { r.Principal.Roles = []string{""} },
		"group":        func(r *ExecutionRecord) { r.Principal.Groups = []string{" "} },
		"limitation":   func(r *ExecutionRecord) { r.Limitations = []Limitation{{}} },
		"provenance":   func(r *ExecutionRecord) { r.Provenance.Source = "" },
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
}

func TestFingerprintRecordset_DeterministicAndOrderSensitive(t *testing.T) {
	recordset := Recordset{
		Columns: []Column{{Name: "id", Type: "integer"}, {Name: "name", Type: "string"}},
		Rows: [][]TypedValue{
			{NewIntegerValue("1"), NewStringValue("one")},
			{NewIntegerValue("2"), NewStringValue("two")},
		},
	}
	first, err := FingerprintRecordset(recordset)
	if err != nil {
		t.Fatal(err)
	}
	second, err := FingerprintRecordset(recordset)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("identical recordsets differ: %s != %s", first, second)
	}

	canonical := `{"columns":[{"name":"id","type":"integer"},{"name":"name","type":"string"}],"rows":[[{"type":"integer","value":"1"},{"type":"string","value":"one"}],[{"type":"integer","value":"2"},{"type":"string","value":"two"}]]}`
	wantDigest := sha256.Sum256([]byte(canonical))
	if want := hex.EncodeToString(wantDigest[:]); first != want {
		t.Fatalf("fingerprint = %s, want canonical-shape hash %s", first, want)
	}

	reordered := recordset
	reordered.Rows = [][]TypedValue{recordset.Rows[1], recordset.Rows[0]}
	different, err := FingerprintRecordset(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if different == first {
		t.Fatal("row-order change should change fingerprint")
	}
}

func TestFingerprintRecordset_RejectsInvalidRecordset(t *testing.T) {
	invalid := Recordset{Columns: []Column{{Name: "id", Type: "integer"}}, Rows: [][]TypedValue{{}}}
	if fingerprint, err := FingerprintRecordset(invalid); err == nil || fingerprint != "" {
		t.Fatalf("invalid recordset fingerprint = %q, err = %v", fingerprint, err)
	}
}

func TestExecutionListContracts(t *testing.T) {
	limit := 100
	request := ExecutionListRequest{
		Scope:   Scope{StoreID: "ops", Project: "billing", Environment: "production", SecurityContextID: "sc-1"},
		QueryID: "invoices/stuck", IncidentID: "INC-1", Since: "2026-09-13T09:00:00Z",
		Until: "2026-09-13T11:00:00Z", Limit: &limit,
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid list request rejected: %v", err)
	}
	for name, mutate := range map[string]func(*ExecutionListRequest){
		"scope":    func(r *ExecutionListRequest) { r.Project = "" },
		"query":    func(r *ExecutionListRequest) { r.QueryID = " invoices/stuck " },
		"incident": func(r *ExecutionListRequest) { r.IncidentID = "bad/id" },
		"since":    func(r *ExecutionListRequest) { r.Since = "bad" },
		"until":    func(r *ExecutionListRequest) { r.Until = "bad" },
		"range":    func(r *ExecutionListRequest) { r.Until = "2026-09-13T08:00:00Z" },
		"limit":    func(r *ExecutionListRequest) { zero := 0; r.Limit = &zero },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := request
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}

	response := ExecutionListResponse{Executions: []ExecutionRecordBrief{validExecutionRecordBrief()}, Truncated: false}
	if err := response.Validate(); err != nil {
		t.Fatalf("valid list response rejected: %v", err)
	}
	response.Executions[0].ID = ""
	if err := response.Validate(); err == nil {
		t.Fatal("invalid brief should be rejected")
	}
}

func TestExecutionRecordBrief_Validation(t *testing.T) {
	brief := validExecutionRecordBrief()
	if err := brief.Validate(); err != nil {
		t.Fatalf("valid brief rejected: %v", err)
	}
	brief.Incident.IncidentID = "bad/id"
	if err := brief.Validate(); err == nil {
		t.Fatal("invalid incident ref should be rejected")
	}
}

func TestSnapshotReadResponse_IsEvidenceNotResult(t *testing.T) {
	recordset := validResult().Recordset
	response := SnapshotReadResponse{RecordID: "exec-1", Recordset: &recordset, Limitations: []Limitation{}, Truncated: false}
	if err := response.Validate(); err != nil {
		t.Fatalf("valid snapshot evidence rejected: %v", err)
	}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"provenance", "bindingsApplied"} {
		if _, ok := generic[forbidden]; ok {
			t.Fatalf("snapshot evidence must not contain %s: %s", forbidden, data)
		}
	}

	expired := SnapshotReadResponse{RecordID: "exec-1", SnapshotExpiredAt: "2026-10-13T10:00:00Z"}
	if err := expired.Validate(); err != nil {
		t.Fatalf("explicit expired evidence rejected: %v", err)
	}
	if err := (SnapshotReadResponse{RecordID: "exec-1"}).Validate(); err == nil {
		t.Fatal("response without rows or explicit expiry should be rejected")
	}
	if err := (SnapshotReadResponse{RecordID: "bad/id", Recordset: &recordset}).Validate(); err == nil {
		t.Fatal("invalid recordId should be rejected")
	}
	invalidExpired := expired
	invalidExpired.Recordset = &recordset
	if err := invalidExpired.Validate(); err == nil {
		t.Fatal("expired response carrying rows should be rejected")
	}
	invalidExpired = expired
	invalidExpired.SnapshotExpiredAt = "bad"
	if err := invalidExpired.Validate(); err == nil {
		t.Fatal("invalid expiry timestamp should be rejected")
	}
	invalidRows := response
	invalidRows.Recordset = &Recordset{Columns: []Column{{Name: "id", Type: "integer"}}, Rows: [][]TypedValue{{}}}
	if err := invalidRows.Validate(); err == nil {
		t.Fatal("invalid recordset should be rejected")
	}
	invalidLimitation := response
	invalidLimitation.Limitations = []Limitation{{}}
	if err := invalidLimitation.Validate(); err == nil {
		t.Fatal("invalid current limitation should be rejected")
	}
}

func TestExecutionSeriesContracts(t *testing.T) {
	request := ExecutionSeriesRequest{
		Scope:   Scope{StoreID: "ops", Project: "billing", Environment: "production", SecurityContextID: "sc-1"},
		QueryID: "invoices/stuck", BindingsApplied: []Binding{}, Projection: ExecutionSeriesProjection{},
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("rowCount series request rejected: %v", err)
	}
	for _, aggregate := range []string{
		SeriesAggregateFirst, SeriesAggregateSum, SeriesAggregateMin,
		SeriesAggregateMax, SeriesAggregateAvg, SeriesAggregateCount,
	} {
		candidate := request
		candidate.Projection = ExecutionSeriesProjection{Column: "Total", Aggregate: aggregate}
		if err := candidate.Validate(); err != nil {
			t.Errorf("aggregate %q rejected: %v", aggregate, err)
		}
	}

	invalidProjection := request
	invalidProjection.Projection = ExecutionSeriesProjection{Column: "Total"}
	if err := invalidProjection.Validate(); err == nil {
		t.Fatal("column without aggregate should be rejected")
	}
	invalidProjection.Projection = ExecutionSeriesProjection{Column: "Total", Aggregate: "median"}
	if err := invalidProjection.Validate(); err == nil {
		t.Fatal("unknown aggregate should be rejected")
	}
	invalidRequest := request
	invalidRequest.QueryID = ""
	if err := invalidRequest.Validate(); err == nil {
		t.Fatal("missing queryId should be rejected")
	}
	invalidRequest.QueryID = " invoices/stuck "
	if err := invalidRequest.Validate(); err == nil {
		t.Fatal("non-canonical queryId should be rejected")
	}
	invalidRequest = request
	invalidRequest.BindingsApplied = []Binding{{}}
	if err := invalidRequest.Validate(); err == nil {
		t.Fatal("invalid binding should be rejected")
	}
	invalidRequest = request
	invalidRequest.Project = ""
	if err := invalidRequest.Validate(); err == nil {
		t.Fatal("invalid scope should be rejected")
	}

	response := ExecutionSeriesResponse{Points: []ExecutionSeriesPoint{
		{ExecutedAt: "2026-09-13T10:00:00Z", Value: NewIntegerValue("37"), RecordID: "exec-1"},
		{ExecutedAt: "2026-09-13T11:00:00Z", Value: NewIntegerValue("19"), RecordID: "exec-2"},
	}, OmittedCount: 1}
	if err := response.Validate(); err != nil {
		t.Fatalf("valid series response rejected: %v", err)
	}

	for name, mutate := range map[string]func(*ExecutionSeriesResponse){
		"omitted":         func(r *ExecutionSeriesResponse) { r.OmittedCount = -1 },
		"point timestamp": func(r *ExecutionSeriesResponse) { r.Points[0].ExecutedAt = "bad" },
		"point value":     func(r *ExecutionSeriesResponse) { r.Points[0].Value = NewIntegerValue("bad") },
		"point id":        func(r *ExecutionSeriesResponse) { r.Points[0].RecordID = "" },
		"order": func(r *ExecutionSeriesResponse) {
			r.Points[0], r.Points[1] = r.Points[1], r.Points[0]
		},
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
