package apicontract

import (
	"encoding/json"
	"sort"
	"strconv"
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
		"hidden one-sided column": func(r *CompareResult) {
			r.Left.Limitations = []Limitation{{Policy: "masked", HiddenColumns: []string{"email"}}}
			r.Summary.ColumnsOnlyOnOneSide = []CompareOneSidedColumn{{Column: "email", Side: CompareColumnRight}}
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
		"duplicate value":      func(r *CompareResult) { r.Distribution.Values[1].Value = r.Distribution.Values[0].Value },
		"pct mismatch":         func(r *CompareResult) { r.Distribution.Values[0].Right.Pct = 99 },
		"ratio mismatch":       func(r *CompareResult) { wrong := float64(2); r.Distribution.Values[0].Ratio = &wrong },
		"declared column type": func(r *CompareResult) { r.Distribution.Values[0].Value = NewIntegerValue("1") },
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

func compareResultWithAdded(ids ...string) CompareResult {
	left := CompareSideReceipt{Execution: ExecutionRef{StoreID: "evidence", ProjectID: "orders", ExecutionID: "left"}, ExecutedAt: "2026-09-14T10:00:00Z", RowCount: 0, Limitations: []Limitation{}, Reproducible: true}
	right := CompareSideReceipt{Execution: ExecutionRef{StoreID: "evidence", ProjectID: "orders", ExecutionID: "right"}, ExecutedAt: "2026-09-14T10:01:00Z", RowCount: len(ids), Limitations: []Limitation{}, Reproducible: true}
	rows := make([]CompareRow, 0, len(ids))
	for _, id := range ids {
		value := NewIntegerValue(id)
		rows = append(rows, CompareRow{Key: []TypedValue{value}, Row: []TypedValue{value}})
	}
	return CompareResult{Left: left, Right: right, Columns: []Column{{Name: "id", Type: "integer"}}, Key: []string{"id"}, Added: rows, Removed: []CompareRow{}, Changed: []CompareChangedRow{}, Summary: CompareSummary{Added: len(ids), ColumnsOnlyOnOneSide: []CompareOneSidedColumn{}}}
}

func TestCompareResultValidateColumnTypesKeyIdentityAndOrder(t *testing.T) {
	valid := compareResultWithAdded("1", "2")
	require.NoError(t, valid.Validate())

	mistyped := compareResultWithAdded("1")
	mistyped.Added[0].Row[0] = NewStringValue("1")
	require.ErrorContains(t, mistyped.Validate(), "declared column type")

	mismatchedKey := compareResultWithAdded("1")
	mismatchedKey.Added[0].Key[0] = NewIntegerValue("2")
	require.ErrorContains(t, mismatchedKey.Validate(), "does not match row")
	nullKey := compareResultWithAdded("1")
	nullKey.Added[0].Key[0], nullKey.Added[0].Row[0] = NewNullValue(), NewNullValue()
	require.ErrorContains(t, nullKey.Validate(), "null/missing key")

	duplicate := compareResultWithAdded("1", "1")
	require.ErrorContains(t, duplicate.Validate(), "duplicate key")

	unstable := compareResultWithAdded("2", "1")
	require.ErrorContains(t, unstable.Validate(), "stable key order")

	crossCategory := compareResultWithAdded("1")
	crossCategory.Removed = []CompareRow{{Key: []TypedValue{NewIntegerValue("1")}, Row: []TypedValue{NewIntegerValue("1")}}}
	crossCategory.Summary.Removed = 1
	crossCategory.Left.RowCount = 1
	require.ErrorContains(t, crossCategory.Validate(), "duplicate key")
}

func TestCompareResultValidateBoundsAndTruncation(t *testing.T) {
	ids := make([]string, CompareMaximumLimit+1)
	for i := range ids {
		ids[i] = strconv.Itoa(i)
	}
	overLimit := compareResultWithAdded(ids...)
	require.ErrorContains(t, overLimit.Validate(), "maximum")

	falseTruncation := validCompareResultForValidation()
	falseTruncation.Truncated = true
	require.ErrorContains(t, falseTruncation.Validate(), "truncated")

	shortDistribution := validDistributionResult()
	shortDistribution.Distribution.Truncated = true
	require.ErrorContains(t, shortDistribution.Validate(), "exactly")

	completeCapped := compareResultWithAdded()
	completeCapped.Left.RowCount, completeCapped.Right.RowCount = CompareDistributionMaximumValues, CompareDistributionMaximumValues
	completeCapped.Summary.Unchanged = CompareDistributionMaximumValues
	one := float64(1)
	completeCapped.Distribution = &CompareDistribution{Column: "id", Truncated: true}
	for i := 0; i < CompareDistributionMaximumValues; i++ {
		completeCapped.Distribution.Values = append(completeCapped.Distribution.Values, CompareDistributionValue{Value: NewIntegerValue(strconv.Itoa(i)), Left: CompareDistributionSide{Count: 1, Pct: 2}, Right: CompareDistributionSide{Count: 1, Pct: 2}, Ratio: &one})
	}
	sort.Slice(completeCapped.Distribution.Values, func(i, j int) bool {
		return compareTypedStableKey(completeCapped.Distribution.Values[i].Value) < compareTypedStableKey(completeCapped.Distribution.Values[j].Value)
	})
	require.ErrorContains(t, completeCapped.Validate(), "omitted")
}
