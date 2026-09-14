package apicontract

import (
	"encoding/json"
	"testing"

	"github.com/datatug/datatug-core/pkg/incidents"
	"github.com/stretchr/testify/require"
)

func validCompareRequest() CompareRequest {
	return CompareRequest{
		SecurityContextID: "ctx-1", QueryID: "orders-by-state",
		Left:  CompareSideSpec{Kind: CompareSideScope, StoreID: "local", Project: "orders", Environment: "prod"},
		Right: CompareSideSpec{Kind: CompareSideScope, StoreID: "local", Project: "orders", Environment: "staging"},
	}
}

func TestCompareRequestValidate(t *testing.T) {
	base := validCompareRequest()
	require.NoError(t, base.Validate())

	tests := map[string]func(*CompareRequest){
		"query required": func(r *CompareRequest) { r.QueryID = "" },
		"facts require incident": func(r *CompareRequest) {
			r.Left = CompareSideSpec{Kind: CompareSideFacts, StoreID: "local", Project: "orders", Environment: "prod", CohortRole: CompareCohortAffected}
		},
		"incident requires mutation": func(r *CompareRequest) { r.Incident = &IncidentRef{StoreID: "incidents", IncidentID: "INC-8"} },
		"mutation requires incident": func(r *CompareRequest) { r.MutationID = "mutation-8" },
		"record requires key": func(r *CompareRequest) {
			r.Left = CompareSideSpec{Kind: CompareSideRecord, Execution: &ExecutionRef{StoreID: "evidence", ProjectID: "orders", ExecutionID: "exec-1"}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) { candidate := base; mutate(&candidate); require.Error(t, candidate.Validate()) })
	}

	withIncident := base
	withIncident.Incident = &IncidentRef{StoreID: "incidents", IncidentID: "INC-8"}
	withIncident.MutationID = "mutation-8"
	withIncident.Left = CompareSideSpec{Kind: CompareSideFacts, StoreID: "local", Project: "orders", Environment: "prod", CohortRole: CompareCohortControl}
	require.NoError(t, withIncident.Validate())
}

func TestCompareRequestDecodeStrictRejectsCheckAndDiscriminatorLeakage(t *testing.T) {
	valid := `{"securityContextId":"ctx-1","queryId":"q-1","left":{"kind":"scope","storeId":"s","project":"p","environment":"e"},"right":{"kind":"scope","storeId":"s","project":"p","environment":"e"}}`
	var request CompareRequest
	require.NoError(t, DecodeStrict([]byte(valid), &request))
	require.NoError(t, request.Validate())

	for _, candidate := range []string{
		`{"securityContextId":"ctx-1","queryId":"q-1","checkId":"c-1","left":{"kind":"scope","storeId":"s","project":"p","environment":"e"},"right":{"kind":"scope","storeId":"s","project":"p","environment":"e"}}`,
		`{"securityContextId":"ctx-1","queryId":"q-1","left":{"kind":"scope","storeId":"s","project":"p","environment":"e","cohortRole":"affected"},"right":{"kind":"scope","storeId":"s","project":"p","environment":"e"}}`,
		`{"securityContextId":"ctx-1","queryId":"q-1","left":{"kind":"facts","storeId":"s","project":"p","environment":"e","cohortRole":"healthy_control"},"right":{"kind":"scope","storeId":"s","project":"p","environment":"e"},"incident":{"storeId":"i","incidentId":"INC-8"},"mutationId":"m-8"}`,
		`{"securityContextId":"ctx-1","queryId":"q-1","left":{"kind":"record","execution":{"storeId":"s","projectId":"p","executionId":"x"},"storeId":"leak"},"right":{"kind":"scope","storeId":"s","project":"p","environment":"e"},"key":["id"]}`,
		`{"securityContextId":"ctx-1","queryId":"q-1","left":{"kind":"facts","storeId":"s","project":"p","environment":"e","cohortRole":"affected","parameters":{}},"right":{"kind":"scope","storeId":"s","project":"p","environment":"e"},"incident":{"storeId":"i","incidentId":"INC-8"},"mutationId":"m-8"}`,
	} {
		require.Error(t, DecodeStrict([]byte(candidate), &request), candidate)
	}
}

func TestCompareErrorResponseCarriesPartialReceipts(t *testing.T) {
	receipt := CompareSideReceipt{Execution: ExecutionRef{StoreID: "evidence", ProjectID: "orders", ExecutionID: "exec-1"}, ExecutedAt: "2026-09-14T10:00:00Z", RowCount: 4, Reproducible: true}
	response := CompareErrorResponse{Error: ErrorBody{Code: string(ErrCodeSourceUnavailable), Message: "right side unavailable", RequestID: "req-8"}, Left: &receipt}
	require.NoError(t, response.Validate())
	wire, err := json.Marshal(response)
	require.NoError(t, err)
	require.Contains(t, string(wire), `"left"`)
	require.NotContains(t, string(wire), `"right"`)

	comparison := incidents.ComparisonRef{Left: receipt.Execution, Right: ExecutionRef{StoreID: "evidence", ProjectID: "orders", ExecutionID: "exec-2"}}
	response.Comparison = &comparison
	require.NoError(t, response.Validate())
}

func validCompareResultForValidation() CompareResult {
	left := CompareSideReceipt{Execution: ExecutionRef{StoreID: "evidence", ProjectID: "orders", ExecutionID: "left"}, ExecutedAt: "2026-09-14T10:00:00Z", RowCount: 1, Limitations: []Limitation{}, Reproducible: true}
	right := CompareSideReceipt{Execution: ExecutionRef{StoreID: "evidence", ProjectID: "orders", ExecutionID: "right"}, ExecutedAt: "2026-09-14T10:01:00Z", RowCount: 1, Limitations: []Limitation{}, Reproducible: true}
	return CompareResult{
		Left: left, Right: right,
		Columns: []Column{{Name: "id", Type: "integer"}, {Name: "name", Type: "string"}}, Key: []string{"id"},
		Added: []CompareRow{}, Removed: []CompareRow{},
		Changed: []CompareChangedRow{{Key: []TypedValue{NewIntegerValue("1")}, Columns: []CompareColumnChange{{Column: "name", Left: NewStringValue("old"), Right: NewStringValue("new")}}}},
		Summary: CompareSummary{Changed: 1, ColumnsOnlyOnOneSide: []CompareOneSidedColumn{}},
	}
}

func TestCompareResultValidateRejectsMalformedDiffs(t *testing.T) {
	require.NoError(t, validCompareResultForValidation().Validate())
	tests := map[string]func(*CompareResult){
		"duplicate shared column": func(r *CompareResult) { r.Columns[1] = r.Columns[0] },
		"invalid shared type":     func(r *CompareResult) { r.Columns[1].Type = "json" },
		"key length":              func(r *CompareResult) { r.Changed[0].Key = nil },
		"row length": func(r *CompareResult) {
			r.Changed = []CompareChangedRow{}
			r.Added = []CompareRow{{Key: []TypedValue{NewIntegerValue("2")}, Row: []TypedValue{NewIntegerValue("2")}}}
			r.Summary.Added, r.Summary.Changed, r.Right.RowCount = 1, 0, 1
		},
		"negative count": func(r *CompareResult) { r.Summary.Unchanged = -1 },
		"invalid side label": func(r *CompareResult) {
			r.Summary.ColumnsOnlyOnOneSide = []CompareOneSidedColumn{{Column: "extra", Side: "both"}}
		},
		"invalid typed value": func(r *CompareResult) { r.Changed[0].Columns[0].Right = TypedValue{Type: "json"} },
		"changed key column":  func(r *CompareResult) { r.Changed[0].Columns[0].Column = "id" },
		"unchanged change":    func(r *CompareResult) { r.Changed[0].Columns[0].Right = NewStringValue("old") },
		"negative distribution count": func(r *CompareResult) {
			r.Distribution = &CompareDistribution{Column: "name", Values: []CompareDistributionValue{{Value: NewStringValue("old"), Left: CompareDistributionSide{Count: -1}, Right: CompareDistributionSide{}, Ratio: nil}}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := validCompareResultForValidation()
			mutate(&candidate)
			require.Error(t, candidate.Validate())
		})
	}
}

func validDistributionResult() CompareResult {
	result := validCompareResultForValidation()
	zero := float64(0)
	result.Distribution = &CompareDistribution{Column: "name", Values: []CompareDistributionValue{
		{Value: NewStringValue("new"), Left: CompareDistributionSide{Count: 0, Pct: 0}, Right: CompareDistributionSide{Count: 1, Pct: 100}, Ratio: &zero},
		{Value: NewStringValue("old"), Left: CompareDistributionSide{Count: 1, Pct: 100}, Right: CompareDistributionSide{Count: 0, Pct: 0}, Ratio: nil},
	}}
	return result
}

func TestCompareResultValidateDistributionSemantics(t *testing.T) {
	require.NoError(t, validDistributionResult().Validate())
	tests := map[string]func(*CompareResult){
		"unstable order": func(r *CompareResult) {
			r.Distribution.Values[0], r.Distribution.Values[1] = r.Distribution.Values[1], r.Distribution.Values[0]
		},
		"duplicate value": func(r *CompareResult) { r.Distribution.Values[1].Value = r.Distribution.Values[0].Value },
		"pct mismatch":    func(r *CompareResult) { r.Distribution.Values[0].Right.Pct = 99 },
		"ratio mismatch":  func(r *CompareResult) { wrong := float64(2); r.Distribution.Values[0].Ratio = &wrong },
		"count mismatch": func(r *CompareResult) {
			r.Distribution.Values[0].Right.Count = 0
			r.Distribution.Values[0].Right.Pct = 0
			r.Distribution.Values[0].Ratio = nil
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := validDistributionResult()
			mutate(&candidate)
			require.Error(t, candidate.Validate())
		})
	}
}
