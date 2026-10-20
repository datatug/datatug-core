package apicontract

import (
	"encoding/json"
	"testing"

	"github.com/datatug/datatug-core/pkg/incidents"
	"github.com/stretchr/testify/require"
)

func resultWithRowEvidence(t *testing.T) Result {
	t.Helper()
	r := validResult()
	r.Execution = &ExecutionRef{StoreID: "firestore", ProjectID: "training", ExecutionID: "query-1"}
	r.Recordset = Recordset{Columns: []Column{{Name: "key", Type: "string"}, {Name: "evidence", Type: "string"}}, Rows: [][]TypedValue{{NewStringValue("42"), NewNullValue()}, {NewIntegerValue("42"), NewNullValue()}}}
	fingerprint, err := FingerprintRecordset(r.Recordset)
	require.NoError(t, err)
	r.RowEvidence = &RowEvidence{Version: 1, Execution: *r.Execution, RecordsetFingerprint: fingerprint, Scope: RowEvidenceScope{StoreID: "firestore", SpaceID: "space", ProjectID: "training", Environment: "demo"}, Source: RowEvidenceSource{ID: "source", Generation: "g1"}, RowKeyColumn: "key", Rows: []RowEvidenceRow{{RowKey: NewStringValue("42"), Slots: map[string]RowEvidenceSlot{"evidence": {State: "available", Refs: []incidents.ArtifactRef{{Kind: incidents.RefExecution, Execution: r.Execution}}}}}, {RowKey: NewIntegerValue("42"), Slots: map[string]RowEvidenceSlot{"evidence": {State: "unavailable"}}}}}
	return r
}
func selectorFor(r Result) RowEvidenceSelector {
	e := r.RowEvidence
	return RowEvidenceSelector{Execution: e.Execution, RecordsetFingerprint: e.RecordsetFingerprint, Scope: e.Scope, Source: e.Source, RowKeyColumn: e.RowKeyColumn, RowKey: NewStringValue("42"), SlotColumn: "evidence"}
}
func TestRowEvidenceRoundTripAndSelection(t *testing.T) {
	r := resultWithRowEvidence(t)
	require.NoError(t, r.Validate())
	data, err := json.Marshal(r)
	require.NoError(t, err)
	var decoded Result
	require.NoError(t, DecodeStrict(data, &decoded))
	require.NoError(t, decoded.Validate())
	require.Equal(t, r, decoded)
	selector := selectorFor(decoded)
	data, err = json.Marshal(selector)
	require.NoError(t, err)
	var roundTrip RowEvidenceSelector
	require.NoError(t, DecodeStrict(data, &roundTrip))
	require.Equal(t, selector, roundTrip)
	slot, available, err := decoded.ResolveRowEvidence(selector)
	require.NoError(t, err)
	require.True(t, available)
	slot.Refs[0].Execution.ExecutionID = "mutated"
	require.Equal(t, "query-1", decoded.RowEvidence.Rows[0].Slots["evidence"].Refs[0].Execution.ExecutionID)
	selector.RowKey = NewIntegerValue("42")
	_, available, err = decoded.ResolveRowEvidence(selector)
	require.NoError(t, err)
	require.False(t, available)
	selector.RowKey = NewStringValue("missing")
	_, available, err = decoded.ResolveRowEvidence(selector)
	require.NoError(t, err)
	require.False(t, available)
	selector = selectorFor(decoded)
	selector.SlotColumn = "absent"
	_, available, err = decoded.ResolveRowEvidence(selector)
	require.NoError(t, err)
	require.False(t, available)
	decoded.RowEvidence = nil
	require.NoError(t, decoded.Validate())
	_, available, err = decoded.ResolveRowEvidence(selector)
	require.NoError(t, err)
	require.False(t, available)
}
func TestRowEvidenceRejectsMalformedMetadata(t *testing.T) {
	cases := map[string]func(*Result){
		"version":              func(r *Result) { r.RowEvidence.Version = 2 },
		"missing execution":    func(r *Result) { r.Execution = nil },
		"wrong execution":      func(r *Result) { r.RowEvidence.Execution.ExecutionID = "other" },
		"invalid execution":    func(r *Result) { r.Execution.ExecutionID = ""; r.RowEvidence.Execution = *r.Execution },
		"invalid project":      func(r *Result) { r.RowEvidence.Scope.ProjectID = "bad/path" },
		"missing environment":  func(r *Result) { r.RowEvidence.Scope.Environment = "" },
		"invalid space":        func(r *Result) { r.RowEvidence.Scope.SpaceID = " space" },
		"missing hosted space": func(r *Result) { r.RowEvidence.Scope.SpaceID = "" },
		"scope":                func(r *Result) { r.RowEvidence.Scope.StoreID = "local" },
		"source":               func(r *Result) { r.RowEvidence.Source.ID = "" },
		"generation":           func(r *Result) { r.RowEvidence.Source.Generation = " g1" },
		"fingerprint":          func(r *Result) { r.RowEvidence.RecordsetFingerprint = "wrong" },
		"invalid recordset":    func(r *Result) { r.Recordset.Rows[0] = nil },
		"bad column": func(r *Result) {
			r.Recordset.Columns[0].Name = " key"
			r.RowEvidence.RecordsetFingerprint, _ = FingerprintRecordset(r.Recordset)
		},
		"duplicate column": func(r *Result) {
			r.Recordset.Columns[1].Name = "key"
			r.RowEvidence.RecordsetFingerprint, _ = FingerprintRecordset(r.Recordset)
		},
		"missing key column": func(r *Result) { r.RowEvidence.RowKeyColumn = "missing" },
		"invalid held key":   func(r *Result) { r.Recordset.Rows[0][0] = TypedValue{Type: "invalid"} },
		"unsupported held key": func(r *Result) {
			r.Recordset.Rows[0][0] = NewNullValue()
			r.RowEvidence.RecordsetFingerprint, _ = FingerprintRecordset(r.Recordset)
		},
		"duplicate held key": func(r *Result) {
			r.Recordset.Rows[1][0] = r.Recordset.Rows[0][0]
			r.RowEvidence.RecordsetFingerprint, _ = FingerprintRecordset(r.Recordset)
		},
		"invalid sidecar key":     func(r *Result) { r.RowEvidence.Rows[0].RowKey = TypedValue{Type: "invalid"} },
		"unsupported sidecar key": func(r *Result) { r.RowEvidence.Rows[0].RowKey = NewNullValue() },
		"unknown key":             func(r *Result) { r.RowEvidence.Rows[0].RowKey = NewStringValue("unknown") },
		"duplicate sidecar key":   func(r *Result) { r.RowEvidence.Rows[1].RowKey = r.RowEvidence.Rows[0].RowKey },
		"slot column":             func(r *Result) { r.RowEvidence.Rows[0].Slots["absent"] = RowEvidenceSlot{State: "unavailable"} },
		"unavailable refs": func(r *Result) {
			r.RowEvidence.Rows[0].Slots["evidence"] = RowEvidenceSlot{State: "unavailable", Refs: []incidents.ArtifactRef{{}}}
		},
		"invalid ref": func(r *Result) {
			r.RowEvidence.Rows[0].Slots["evidence"] = RowEvidenceSlot{State: "available", Refs: []incidents.ArtifactRef{{}}}
		},
		"missing refs": func(r *Result) { r.RowEvidence.Rows[0].Slots["evidence"] = RowEvidenceSlot{State: "available"} },
		"foreign retained execution": func(r *Result) {
			other := *r.Execution
			other.ExecutionID = "other"
			r.RowEvidence.Rows[0].Slots["evidence"] = RowEvidenceSlot{State: "available", Refs: []incidents.ArtifactRef{{Kind: incidents.RefExecution, Execution: &other}}}
		},
		"wrong state": func(r *Result) { r.RowEvidence.Rows[0].Slots["evidence"] = RowEvidenceSlot{State: "pass"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := resultWithRowEvidence(t)
			mutate(&r)
			require.Error(t, r.RowEvidence.ValidateForResult(r))
		})
	}
}
func TestRowEvidenceSelectorMismatch(t *testing.T) {
	for name, mutate := range map[string]func(*RowEvidenceSelector){
		"execution": func(s *RowEvidenceSelector) { s.Execution.ExecutionID = "other" }, "fingerprint": func(s *RowEvidenceSelector) { s.RecordsetFingerprint = "other" }, "scope": func(s *RowEvidenceSelector) { s.Scope.SpaceID = "other" }, "generation": func(s *RowEvidenceSelector) { s.Source.Generation = "g2" }, "key column": func(s *RowEvidenceSelector) { s.RowKeyColumn = "other" }, "key type": func(s *RowEvidenceSelector) { s.RowKey = NewBooleanValue(true) }, "slot": func(s *RowEvidenceSelector) { s.SlotColumn = "" },
	} {
		t.Run(name, func(t *testing.T) {
			r := resultWithRowEvidence(t)
			s := selectorFor(r)
			mutate(&s)
			_, available, err := r.ResolveRowEvidence(s)
			require.Error(t, err)
			require.False(t, available)
		})
	}
	r := resultWithRowEvidence(t)
	s := selectorFor(r)
	r.RowEvidence.Version = 2
	_, _, err := r.ResolveRowEvidence(s)
	require.Error(t, err)
}
func TestRowEvidenceStrictDecoding(t *testing.T) {
	for _, data := range []string{`{"version":1,"unknown":true}`, `{"version":1,"version":1}`, `{"version":1,"rows":[{"rowKey":{"type":"string","value":"x"},"slots":{"slot":{"state":"available","refs":[],"sql":"SELECT secret"}}}]}`} {
		var e RowEvidence
		require.Error(t, json.Unmarshal([]byte(data), &e))
	}
	r := resultWithRowEvidence(t)
	r.RowEvidence.Rows = nil
	require.NoError(t, r.Validate())
	r.RowEvidence.Scope.StoreID = "local"
	r.Execution.StoreID = "local"
	r.RowEvidence.Execution = *r.Execution
	r.RowEvidence.Scope.SpaceID = ""
	require.NoError(t, r.Validate())
}

func TestResultEvidenceWirePresence(t *testing.T) {
	r := validResult()
	data, err := json.Marshal(r)
	require.NoError(t, err)
	var decoded Result
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Nil(t, decoded.RowEvidence)
	for _, suffix := range []string{`,"rowEvidence":null}`, `,"RowEvidence":{}}`, `,"rowEvidence":{},"rowEvidence":{}}`, `,"rowEvidence":{"unknown":true}}`} {
		malformed := string(data[:len(data)-1]) + suffix
		require.Error(t, json.Unmarshal([]byte(malformed), &decoded))
	}
	require.Error(t, json.Unmarshal([]byte(`{"recordset":1}`), &decoded))
}
