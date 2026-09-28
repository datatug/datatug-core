package apicontract

import (
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/incidents"
	"github.com/stretchr/testify/assert"
)

func TestCoverage_TypedKeyOrder(t *testing.T) {
	// CompareTypedKeys: length mismatch
	_, err := CompareTypedKeys([]TypedValue{{Type: ValueTypeString, Str: "a"}}, nil)
	assert.Error(t, err)

	// type mismatch
	_, err = CompareTypedKeys([]TypedValue{{Type: ValueTypeString, Str: "a"}}, []TypedValue{{Type: ValueTypeInteger, Str: "1"}})
	assert.Error(t, err)

	// left invalid
	_, err = CompareTypedKeys([]TypedValue{{Type: ValueTypeNull}}, []TypedValue{{Type: ValueTypeNull}})
	assert.Error(t, err)

	// right invalid
	_, err = CompareTypedKeys([]TypedValue{{Type: ValueTypeString, Str: "a"}}, []TypedValue{{Type: ValueTypeString, Str: ""}})
	// Wait, empty string is valid for ValueTypeString, but let's make invalid datetime
	_, err = CompareTypedKeys(
		[]TypedValue{{Type: ValueTypeDatetime, Str: "2020-01-01T00:00:00Z"}},
		[]TypedValue{{Type: ValueTypeDatetime, Str: "invalid-time"}},
	)
	assert.Error(t, err)

	// order != 0 and order == 0 for various types
	types := []struct {
		left, right TypedValue
		expected    int
	}{
		{TypedValue{Type: ValueTypeString, Str: "a"}, TypedValue{Type: ValueTypeString, Str: "b"}, -1},
		{TypedValue{Type: ValueTypeString, Str: "b"}, TypedValue{Type: ValueTypeString, Str: "a"}, 1},
		{TypedValue{Type: ValueTypeString, Str: "a"}, TypedValue{Type: ValueTypeString, Str: "a"}, 0},

		{TypedValue{Type: ValueTypeNumber, Num: 0}, TypedValue{Type: ValueTypeNumber, Num: 0}, 0},
		{TypedValue{Type: ValueTypeNumber, Num: -5.5}, TypedValue{Type: ValueTypeNumber, Num: 5.5}, -1},
		{TypedValue{Type: ValueTypeNumber, Num: 10}, TypedValue{Type: ValueTypeNumber, Num: 2}, 1},

		{TypedValue{Type: ValueTypeInteger, Str: "0"}, TypedValue{Type: ValueTypeInteger, Str: "0"}, 0},
		{TypedValue{Type: ValueTypeInteger, Str: "-5"}, TypedValue{Type: ValueTypeInteger, Str: "5"}, -1},
		{TypedValue{Type: ValueTypeInteger, Str: "10"}, TypedValue{Type: ValueTypeInteger, Str: "2"}, 1},

		{TypedValue{Type: ValueTypeDecimal, Str: "0"}, TypedValue{Type: ValueTypeDecimal, Str: "0.0"}, -1},
		{TypedValue{Type: ValueTypeDecimal, Str: "-1.5"}, TypedValue{Type: ValueTypeDecimal, Str: "1.5"}, -1},
		{TypedValue{Type: ValueTypeDecimal, Str: "1.5"}, TypedValue{Type: ValueTypeDecimal, Str: "1.5"}, 0},

		{TypedValue{Type: ValueTypeBoolean, Bool: false}, TypedValue{Type: ValueTypeBoolean, Bool: true}, -1},
		{TypedValue{Type: ValueTypeBoolean, Bool: true}, TypedValue{Type: ValueTypeBoolean, Bool: false}, 1},
		{TypedValue{Type: ValueTypeBoolean, Bool: true}, TypedValue{Type: ValueTypeBoolean, Bool: true}, 0},

		{TypedValue{Type: ValueTypeDate, Str: "2020-01-01"}, TypedValue{Type: ValueTypeDate, Str: "2020-01-02"}, -1},
		{TypedValue{Type: ValueTypeDatetime, Str: "2020-01-01T00:00:00Z"}, TypedValue{Type: ValueTypeDatetime, Str: "2020-01-01T00:00:00Z"}, 0},
	}
	for _, tc := range types {
		ord, err := CompareTypedKeys([]TypedValue{tc.left}, []TypedValue{tc.right})
		assert.NoError(t, err)
		if tc.expected < 0 {
			assert.Less(t, ord, 0)
		} else if tc.expected > 0 {
			assert.Greater(t, ord, 0)
		} else {
			assert.Equal(t, 0, ord)
		}
	}

	// TypedKeySortKey: empty values
	_, err = TypedKeySortKey(nil)
	assert.Error(t, err)

	// TypedKeySortKey: invalid value
	_, err = TypedKeySortKey([]TypedValue{{Type: ValueTypeNull}})
	assert.Error(t, err)

	// TypedKeySortKey: valid value
	k, err := TypedKeySortKey([]TypedValue{{Type: ValueTypeString, Str: "hello"}})
	assert.NoError(t, err)
	assert.NotEmpty(t, k)

	// TypedValueSortKey
	// invalid
	_, err = TypedValueSortKey(TypedValue{Type: "invalid"})
	assert.Error(t, err)

	// null
	nullKey, err := TypedValueSortKey(TypedValue{Type: ValueTypeNull})
	assert.NoError(t, err)
	assert.Equal(t, "0/", nullKey)

	// non-null valid
	valKey, err := TypedValueSortKey(TypedValue{Type: ValueTypeString, Str: "test"})
	assert.NoError(t, err)
	assert.NotEmpty(t, valKey)

	// sortableTypedValue: invalid type
	_, err = sortableTypedValue(TypedValue{Type: "unknown"})
	assert.Error(t, err)

	// sortableTypedValue: null error
	_, err = sortableTypedValue(TypedValue{Type: ValueTypeNull})
	assert.Error(t, err)
}

func TestCoverage_Binding(t *testing.T) {
	// binding.go:65 (both factId and valueFactIds present)
	err := validateValueFactProvenance(BindingOriginSelection, TypedValueOrSet{Scalar: &TypedValue{Type: ValueTypeString, Str: "a"}}, "f1", [][]string{{"f2"}})
	assert.Error(t, err)

	// binding.go:70 (factId present for set value)
	setVal := TypedValueOrSet{Set: &TypedValueSet{Values: []TypedValue{{Type: ValueTypeString, Str: "a"}}}}
	err = validateValueFactProvenance(BindingOriginSelection, setVal, "f1", nil)
	assert.Error(t, err)

	// binding.go:74 (!fromFacts && valueFactIDs != nil)
	err = validateValueFactProvenance(BindingOriginManual, setVal, "", [][]string{{"f1"}})
	assert.Error(t, err)

	// binding.go:90 (!fromFacts && factID != "")
	scalarVal := TypedValueOrSet{Scalar: &TypedValue{Type: ValueTypeString, Str: "a"}}
	err = validateValueFactProvenance(BindingOriginManual, scalarVal, "f1", nil)
	assert.Error(t, err)

	// binding.go:104 (validateFactID failure in groups)
	err = validateFactIDGroups([][]string{{" invalid "}})
	assert.Error(t, err)

	// binding.go:121 (empty factID)
	err = validateFactID("f", "")
	assert.Error(t, err)

	// binding.go:124 (non-canonical factID)
	err = validateFactID("f", " f1 ")
	assert.Error(t, err)
}

func TestCoverage_Candidate(t *testing.T) {
	// candidate.go:44 (both FactID and ValueFactIDs present)
	c := ChainStep{
		ParameterID:  "p1",
		Explanation:  "test",
		FactID:       "f1",
		ValueFactIDs: [][]string{{"f2"}},
	}
	assert.Error(t, c.Validate())

	// candidate.go:154 (equalFactIDGroups unequal elements)
	g1 := [][]string{{"a", "b"}}
	g2 := [][]string{{"a", "c"}}
	assert.False(t, equalFactIDGroups(g1, g2))
}

func TestCoverage_RelatedRowsRequest(t *testing.T) {
	// related_rows_request.go:42, 45, 48
	req := RelatedRowsRequest{
		LookupID: "lookup",
		Value:    TypedValue{Type: ValueTypeString, Str: "val"},
	}
	assert.Error(t, req.Validate()) // missing project
	req.Project = "proj"
	assert.Error(t, req.Validate()) // missing environment
	req.Environment = "env"
	assert.Error(t, req.Validate()) // missing securityContextId
}

func TestCoverage_ExecutionRequest(t *testing.T) {
	// execution_request.go:23
	entry := BindingOriginEntry{
		ParameterID:  "p1",
		Origin:       BindingOriginSelection,
		FactID:       "f1",
		ValueFactIDs: [][]string{{"f2"}},
	}
	assert.Error(t, entry.Validate())

	// execution_request.go:64 (UnmarshalJSON invalid)
	var er ExecutionRequest
	assert.Error(t, er.UnmarshalJSON([]byte("invalid json")))

	// execution_request.go:68, 80 (duplicate parameter in BindingOrigins)
	dupOrigins := `{"project":"p","environment":"e","securityContextId":"c","queryId":"q","bindingOrigins":[{"parameterId":"p1","origin":"manual"},{"parameterId":"p1","origin":"manual"}]}`
	assert.Error(t, er.UnmarshalJSON([]byte(dupOrigins)))

	// execution_request.go:86 (value.IsSet() false continues)
	erValid := ExecutionRequest{
		Project:           "p",
		Environment:       "e",
		SecurityContextID: "c",
		QueryID:           "q",
		Parameters: map[string]TypedValueOrSet{
			"p1": {Scalar: &TypedValue{Type: ValueTypeString, Str: "val"}},
		},
		BindingOrigins: []BindingOriginEntry{
			{ParameterID: "p1", Origin: BindingOriginManual},
		},
	}
	assert.NoError(t, erValid.Normalize())

	// execution_request.go:95 (NormalizeTypedValueSet error)
	erBadSet := ExecutionRequest{
		Parameters: map[string]TypedValueOrSet{
			"p1": {Set: &TypedValueSet{Values: nil}},
		},
	}
	assert.Error(t, erBadSet.Normalize())

	// execution_request.go:142 (non-canonical key in Parameters)
	erBadKey := validExecutionRequest()
	erBadKey.Parameters = map[string]TypedValueOrSet{
		" bad ": {Scalar: &TypedValue{Type: ValueTypeString, Str: "v"}},
	}
	assert.Error(t, erBadKey.Validate())
}

func validExecutionRequest() ExecutionRequest {
	return ExecutionRequest{
		Project:           "p",
		Environment:       "e",
		SecurityContextID: "c",
		QueryID:           "q",
		Mode:              "live",
	}
}

func TestCoverage_Incidents(t *testing.T) {
	// incidents.go:120 (validateStrictNestedJSON default unknown type returns nil)
	req := IncidentAppendRequest{
		Event: IncidentEventInput{
			Type: "unknown.event.type",
		},
	}
	assert.NoError(t, req.validateStrictNestedJSON())
}

func TestCoverage_TypedValueSet(t *testing.T) {
	// typed_value_set.go:37
	_, _, err := NormalizeTypedValueSet(
		TypedValueSet{Values: []TypedValue{{Type: ValueTypeString, Str: "a"}}},
		[][]string{{"f1"}, {"f2"}},
	)
	assert.Error(t, err)

	// typed_value_set.go:58 (empty group in valueFactIDs)
	_, _, err = NormalizeTypedValueSet(
		TypedValueSet{Values: []TypedValue{{Type: ValueTypeString, Str: "a"}}},
		[][]string{{}},
	)
	assert.Error(t, err)

	// typed_value_set.go:88 (validateFactIDGroups failure: same fact in multiple groups)
	_, _, err = NormalizeTypedValueSet(
		TypedValueSet{Values: []TypedValue{
			{Type: ValueTypeString, Str: "a"},
			{Type: ValueTypeString, Str: "b"},
		}},
		[][]string{{"f1"}, {"f1"}},
	)
	assert.Error(t, err)

	// typed_value_set.go:103 (duplicate keys in UnmarshalJSON)
	var tvs TypedValueSet
	assert.Error(t, tvs.UnmarshalJSON([]byte(`{"type":"set","values":[],"type":"set"}`)))

	// typed_value_set.go:113 (raw.Type != ValueTypeSet)
	assert.Error(t, tvs.UnmarshalJSON([]byte(`{"type":"unknown","values":[]}`)))

	// typed_value_set.go:117 (validateTypedValueSetContent error in UnmarshalJSON)
	assert.Error(t, tvs.UnmarshalJSON([]byte(`{"type":"set","values":[]}`)))

	// typed_value_set.go:125 (TypedValueSet.Validate() error for empty values)
	assert.Error(t, TypedValueSet{}.Validate())

	// typed_value_set.go:207 (TypedValueOrSet.MarshalJSON error when invalid)
	badOrSet := TypedValueOrSet{Scalar: &TypedValue{Type: "bad"}}
	_, err = badOrSet.MarshalJSON()
	assert.Error(t, err)

	// typed_value_set.go:215 (TypedValueOrSet.UnmarshalJSON invalid json for struct)
	var badTvos TypedValueOrSet
	assert.Error(t, badTvos.UnmarshalJSON([]byte(`123`)))

	// typed_value_set.go:227 (bad scalar unmarshal)
	assert.Error(t, badTvos.UnmarshalJSON([]byte(`{"type":"string","num":"not-a-number"}`)))
}

func TestCoverage_ExecutionRecords(t *testing.T) {
	// execution_records.go:61 (GrantRef.Validate incident error)
	gr := GrantRef{Incident: IncidentRef{StoreID: "bad store id!"}}
	assert.Error(t, gr.Validate())

	// execution_records.go:90, 93, 97, 105, 108 (FieldAccessRef.Validate)
	fa := FieldAccessRef{
		StoreID:     "s",
		Project:     "p",
		Environment: "e",
		Source:      " bad ",
		Column:      "col",
	}
	assert.Error(t, fa.Validate()) // non-canonical source

	fa.Source = "source"
	fa.Column = " bad "
	assert.Error(t, fa.Validate()) // non-canonical column

	fa.Column = "col"
	fa.Collection = " bad "
	assert.Error(t, fa.Validate()) // non-canonical collection

	fa.Collection = "coll"
	fa.Entity = " bad "
	fa.Field = "f"
	assert.Error(t, fa.Validate()) // non-canonical entity

	fa.Entity = "e"
	fa.Field = " bad "
	assert.Error(t, fa.Validate()) // non-canonical field

	// execution_records.go:138 (MeasurementProjection.Validate aggregate error)
	mp := MeasurementProjection{ID: "p1", Aggregate: "unknown"}
	assert.Error(t, mp.Validate())

	// execution_records.go:169, 181, 195 (ScalarMeasurement.Validate, validateMeasurementOutcome)
	sm := ScalarMeasurement{
		Projection: MeasurementProjection{ID: " bad "},
	}
	assert.Error(t, sm.Validate())

	sm.Projection = MeasurementProjection{ID: "p1", Aggregate: MeasurementAggregateRowCount}
	sm.Completeness = MeasurementComplete
	sm.Value = &TypedValue{Type: "invalid"}
	assert.Error(t, sm.Validate())

	sm.Completeness = "unknown_completeness"
	assert.Error(t, sm.Validate())

	// execution_records.go:351 (numericTypedValueEqualsInt float path)
	assert.True(t, numericTypedValueEqualsInt(TypedValue{Type: ValueTypeNumber, Num: 42}, 42))

	// execution_records.go:381 (SnapshotState.Validate non-canonical reason)
	ss := SnapshotState{
		Availability: SnapshotExpired,
		ChangedAt:    time.Now().UTC().Format(time.RFC3339),
		Reason:       " bad reason ",
	}
	assert.Error(t, ss.Validate())

	// execution_records.go:549, 561, 565 (SnapshotReadResponse.Validate)
	srr := SnapshotReadResponse{
		Execution:   ExecutionRef{StoreID: "s", ProjectID: "p", ExecutionID: "e"},
		SnapshotRef: "snap1",
		SnapshotState: SnapshotState{
			Availability: "bad",
		},
	}
	assert.Error(t, srr.Validate())

	srr.SnapshotState = SnapshotState{
		Availability: SnapshotAvailable,
		ChangedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	srr.Recordset = &Recordset{
		Columns: []Column{{Name: " bad "}},
	}
	assert.Error(t, srr.Validate())

	srr.Recordset = &Recordset{
		Columns: []Column{{Name: "col", Type: "string"}},
	}
	srr.Limitations = []Limitation{{Policy: ""}}
	assert.Error(t, srr.Validate())

	// execution_records.go:587, 605, 620 (ExecutionSeriesPartition and ExecutionSeriesRequest)
	part := ExecutionSeriesPartition{EvidenceStoreID: " bad "}
	assert.Error(t, part.Validate())

	part.EvidenceStoreID = "store1"
	part.SourceScope = ExecutionRecordScope{StoreID: "store1", Project: "p", Environment: "e"}
	part.Source = "src"
	part.PolicyFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	part.QueryID = "q1"
	part.QueryRevision = "r1"
	part.BindingsApplied = []Binding{{ParameterID: " bad "}}
	assert.Error(t, part.Validate()) // invalid bindings

	part.BindingsApplied = nil
	part.Projection = MeasurementProjection{ID: "p1", Aggregate: MeasurementAggregateRowCount}
	assert.NoError(t, part.Validate())

	req := ExecutionSeriesRequest{
		Scope:     Scope{StoreID: "store1", Project: "p", Environment: "e", SecurityContextID: "ctx"},
		Partition: part,
	}
	assert.NoError(t, req.Validate())

	reqBadScope := ExecutionSeriesRequest{Scope: Scope{}}
	assert.Error(t, reqBadScope.Validate())

	// execution_records.go:710, 718 (validateExecutionQueryIdentity)
	assert.Error(t, validateExecutionQueryIdentity(" bad ", "", ""))
	assert.Error(t, validateExecutionQueryIdentity("q1", " bad ", ""))
}

func TestCoverage_Compare(t *testing.T) {
	// compare.go:48 (CompareSideSpec.UnmarshalJSON invalid json)
	var spec CompareSideSpec
	assert.Error(t, spec.UnmarshalJSON([]byte("not json")))

	// compare.go:78, 81, 85, 88 (CompareSideScope invalid variants)
	scopeSide := CompareSideSpec{
		Kind:        CompareSideScope,
		StoreID:     " bad ",
		Project:     "p",
		Environment: "e",
	}
	assert.Error(t, scopeSide.Validate())

	scopeSide.StoreID = "s"
	scopeSide.CohortRole = "affected"
	assert.Error(t, scopeSide.Validate())

	scopeSide.CohortRole = ""
	scopeSide.Parameters = map[string]TypedValue{" bad ": {Type: ValueTypeString, Str: "v"}}
	assert.Error(t, scopeSide.Validate())

	scopeSide.Parameters = map[string]TypedValue{"p1": {Type: "bad"}}
	assert.Error(t, scopeSide.Validate())

	// compare.go:93, 99 (CompareSideFacts invalid)
	factsSide := CompareSideSpec{
		Kind:        CompareSideFacts,
		StoreID:     " bad ",
		Project:     "p",
		Environment: "e",
		CohortRole:  CompareCohortAffected,
	}
	assert.Error(t, factsSide.Validate())

	factsSide.StoreID = "s"
	factsSide.Parameters = map[string]TypedValue{"p1": {Type: ValueTypeString, Str: "v"}}
	assert.Error(t, factsSide.Validate())

	// compare.go:103, 106, 109 (CompareSideRecord and default kind)
	recordSide := CompareSideSpec{
		Kind:      CompareSideRecord,
		StoreID:   "s", // shouldn't have storeId
		Execution: &ExecutionRef{StoreID: "s", ProjectID: "p", ExecutionID: "e"},
	}
	assert.Error(t, recordSide.Validate())

	recordSide.StoreID = ""
	recordSide.Execution = &ExecutionRef{StoreID: " bad "}
	assert.Error(t, recordSide.Validate())

	badKindSide := CompareSideSpec{Kind: "unknown"}
	assert.Error(t, badKindSide.Validate())

	// compare.go:128, 134, 137, 141, 158, 161, 167, 171 (CompareRequest.Validate)
	cr := CompareRequest{}
	assert.Error(t, cr.Validate()) // missing securityContextId

	cr.SecurityContextID = "ctx"
	cr.QueryID = "q1"
	cr.Left = CompareSideSpec{Kind: "unknown"}
	assert.Error(t, cr.Validate()) // left invalid

	cr.Left = CompareSideSpec{Kind: CompareSideScope, StoreID: "s", Project: "p", Environment: "e"}
	cr.Right = CompareSideSpec{Kind: "unknown"}
	assert.Error(t, cr.Validate()) // right invalid

	cr.Right = cr.Left
	cr.Incident = &IncidentRef{StoreID: " bad "}
	assert.Error(t, cr.Validate()) // incident invalid

	cr.Incident = nil
	cr.Key = []string{" bad "}
	assert.Error(t, cr.Validate()) // key non-canonical

	cr.Key = []string{"k1", "k1"}
	assert.Error(t, cr.Validate()) // duplicate key

	cr.Key = []string{"k1"}
	cr.DistributionColumn = " bad "
	assert.Error(t, cr.Validate()) // non-canonical distributionColumn

	cr.DistributionColumn = "k1"
	badLimit := 0
	cr.Limit = &badLimit
	assert.Error(t, cr.Validate()) // limit < 1

	// compare.go:186, 189, 192, 196 (CompareSideReceipt.Validate)
	csr := CompareSideReceipt{Execution: ExecutionRef{StoreID: " bad "}}
	assert.Error(t, csr.Validate())

	csr.Execution = ExecutionRef{StoreID: "s", ProjectID: "p", ExecutionID: "e"}
	csr.ExecutedAt = "not a timestamp"
	assert.Error(t, csr.Validate())

	csr.ExecutedAt = time.Now().UTC().Format(time.RFC3339)
	csr.RowCount = -1
	assert.Error(t, csr.Validate())

	csr.RowCount = 0
	csr.Limitations = []Limitation{{Policy: ""}}
	assert.Error(t, csr.Validate())

	// compare.go:265, 268 (CompareResult.Validate left/right error)
	res := CompareResult{Left: CompareSideReceipt{Execution: ExecutionRef{StoreID: " bad "}}}
	assert.Error(t, res.Validate())

	res.Left = CompareSideReceipt{
		Execution:  ExecutionRef{StoreID: "s", ProjectID: "p", ExecutionID: "e"},
		ExecutedAt: time.Now().UTC().Format(time.RFC3339),
	}
	res.Right = CompareSideReceipt{Execution: ExecutionRef{StoreID: " bad "}}
	assert.Error(t, res.Validate())

	// compare.go:284, 290, 297, 300 (CompareResult.Validate columns & keys)
	res.Right = res.Left
	res.Columns = []Column{{Name: "b", Type: "string"}, {Name: "a", Type: "string"}} // not in order
	assert.Error(t, res.Validate())

	res.Columns = []Column{{Name: "a", Type: "string"}}
	res.Key = nil // missing key
	assert.Error(t, res.Validate())

	res.Key = []string{"not_shared"}
	assert.Error(t, res.Validate())

	res.Key = []string{"a", "a"}
	assert.Error(t, res.Validate())

	res.Key = []string{"a"}

	// compare.go:321 (validateCompareValues error in Added)
	res.Added = []CompareRow{{Key: nil}}
	assert.Error(t, res.Validate())

	// compare.go:327 (validateCompareColumnTypes error in Added)
	res.Added = []CompareRow{{
		Key: []TypedValue{{Type: ValueTypeInteger, Str: "1"}},
		Row: []TypedValue{{Type: ValueTypeInteger, Str: "1"}},
	}}
	assert.Error(t, res.Validate())
	res.Added = nil

	// compare.go:351, 354, 357, 366, 369, 376 (CompareResult changed row validation)
	res.Changed = []CompareChangedRow{{Key: []TypedValue{{Type: ValueTypeString, Str: "k1"}}}} // no changed columns
	assert.Error(t, res.Validate())

	// compare.go:352 (validateCompareColumnTypes in Changed)
	res.Changed = []CompareChangedRow{{
		Key:     []TypedValue{{Type: ValueTypeInteger, Str: "1"}},
		Columns: []CompareColumnChange{{Column: "a", Left: TypedValue{Type: ValueTypeString, Str: "1"}, Right: TypedValue{Type: ValueTypeString, Str: "2"}}},
	}}
	assert.Error(t, res.Validate())

	// compare.go:355 (validateCompareKey in Changed)
	res.Changed = []CompareChangedRow{{
		Key:     []TypedValue{{Type: ValueTypeNull}},
		Columns: []CompareColumnChange{{Column: "a", Left: TypedValue{Type: ValueTypeString, Str: "1"}, Right: TypedValue{Type: ValueTypeString, Str: "2"}}},
	}}
	assert.Error(t, res.Validate())

	// compare.go:364 (row.Columns not stable in Changed)
	res.Columns = []Column{{Name: "a", Type: "string"}, {Name: "b", Type: "string"}, {Name: "c", Type: "string"}}
	res.Changed = []CompareChangedRow{{
		Key: []TypedValue{{Type: ValueTypeString, Str: "k"}},
		Columns: []CompareColumnChange{
			{Column: "c", Left: TypedValue{Type: ValueTypeString, Str: "1"}, Right: TypedValue{Type: ValueTypeString, Str: "2"}},
			{Column: "b", Left: TypedValue{Type: ValueTypeString, Str: "1"}, Right: TypedValue{Type: ValueTypeString, Str: "2"}},
		},
	}}
	assert.Error(t, res.Validate())

	// compare.go:367 (change.Left.Validate error in Changed)
	res.Changed = []CompareChangedRow{{
		Key: []TypedValue{{Type: ValueTypeString, Str: "k"}},
		Columns: []CompareColumnChange{
			{Column: "b", Left: TypedValue{Type: "bad"}, Right: TypedValue{Type: ValueTypeString, Str: "2"}},
		},
	}}
	assert.Error(t, res.Validate())

	// compare.go:374 (change.Left doesn't match column type in Changed)
	res.Changed = []CompareChangedRow{{
		Key: []TypedValue{{Type: ValueTypeString, Str: "k"}},
		Columns: []CompareColumnChange{
			{Column: "b", Left: TypedValue{Type: ValueTypeInteger, Str: "1"}, Right: TypedValue{Type: ValueTypeString, Str: "2"}},
		},
	}}
	assert.Error(t, res.Validate())

	// compare.go:392 (counts do not reconcile)
	res.Changed = nil
	res.Summary = CompareSummary{Unchanged: 10}
	assert.Error(t, res.Validate())

	// compare.go:400, 412 (ColumnsOnlyOnOneSide validation)
	res.Summary = CompareSummary{
		ColumnsOnlyOnOneSide: []CompareOneSidedColumn{{Column: "a", Side: CompareColumnLeft}}, // "a" is shared
	}
	assert.Error(t, res.Validate())

	res.Summary.ColumnsOnlyOnOneSide = []CompareOneSidedColumn{
		{Column: "z", Side: CompareColumnLeft},
		{Column: "y", Side: CompareColumnLeft}, // not in stable order
	}
	assert.Error(t, res.Validate())

	// compare.go:417 (policyLimited error)
	res.Summary.ColumnsOnlyOnOneSide = nil
	res.PolicyLimited = true // without rowsFiltered limitations
	assert.Error(t, res.Validate())

	// compare.go:421, 424, 430, 437, 452 (Distribution validation)
	res.PolicyLimited = false
	res.Distribution = &CompareDistribution{Column: "not_shared"}
	assert.Error(t, res.Validate())

	res.Distribution.Column = "a"
	res.Distribution.Values = make([]CompareDistributionValue, CompareDistributionMaximumValues+1)
	assert.Error(t, res.Validate())

	res.Distribution.Values = []CompareDistributionValue{{Value: TypedValue{Type: "bad"}}}
	assert.Error(t, res.Validate())

	// compare.go:450 (value.Ratio presence must follow right pct)
	ratio := 1.0
	res.Distribution.Values = []CompareDistributionValue{{
		Value: TypedValue{Type: ValueTypeString, Str: "v"},
		Right: CompareDistributionSide{Pct: 0},
		Ratio: &ratio,
	}}
	assert.Error(t, res.Validate())

	// compare.go:507 (CompareTypedKeys in validateCompareKey error)
	assert.Error(t, validateCompareKey("g", 1, []TypedValue{{Type: ValueTypeInteger, Str: "1"}}, []TypedValue{{Type: ValueTypeString, Str: "s"}}, map[string]string{}))

	// compare.go:477, 509, 523 (validateCompareValues, validateCompareKey, compareKeyIdentity)
	assert.Error(t, validateCompareValues("g", 0, "f", []TypedValue{{Type: "bad"}}, 1))
	seen := map[string]string{}
	// null key error
	assert.Error(t, validateCompareKey("g", 0, []TypedValue{{Type: ValueTypeNull}}, nil, seen))
	// compareKeyIdentity zero float
	id1 := compareKeyIdentity([]TypedValue{{Type: ValueTypeNumber, Num: 0}})
	assert.NotEmpty(t, id1)

	// compare.go:532, 533 (compareRowsFiltered)
	assert.True(t, compareRowsFiltered([]Limitation{{RowsFiltered: true}}))
	assert.False(t, compareRowsFiltered([]Limitation{{RowsFiltered: false}}))

	// compare.go:553 (comparePercentage total == 0)
	assert.Equal(t, float64(0), comparePercentage(5, 0))

	// compare.go:570, 574, 579, 584 (CompareErrorResponse.Validate)
	cer := CompareErrorResponse{
		Error: ErrorBody{Code: string(ErrCodeInvalidRequest), Message: "something failed", RequestID: "req-1"},
	}
	assert.NoError(t, cer.Validate())

	cerBadErr := CompareErrorResponse{Error: ErrorBody{}}
	assert.Error(t, cerBadErr.Validate())

	cer.Left = &CompareSideReceipt{Execution: ExecutionRef{StoreID: " bad "}}
	assert.Error(t, cer.Validate())

	cer.Left = &CompareSideReceipt{
		Execution:  ExecutionRef{StoreID: "s", ProjectID: "p", ExecutionID: "e"},
		ExecutedAt: time.Now().UTC().Format(time.RFC3339),
	}
	cer.Right = &CompareSideReceipt{Execution: ExecutionRef{StoreID: " bad "}}
	assert.Error(t, cer.Validate())

	cer.Right = cer.Left
	cer.Comparison = &incidents.ComparisonRef{Left: incidents.ExecutionRef{ExecutionID: " bad "}}
	assert.Error(t, cer.Validate())
}
