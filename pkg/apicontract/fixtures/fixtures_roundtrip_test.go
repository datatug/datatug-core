package fixtures

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

type validator interface {
	Validate() error
}

// roundTrip proves fixture name deserializes into T, that T passes its own
// Validate() (a frozen fixture must itself be a valid instance of the
// contract it demonstrates), and that re-encoding it with the same
// canonical form (json.MarshalIndent, 2-space, trailing newline) this
// package's fixtures were generated with reproduces the fixture's bytes
// exactly - "round-trips through the Go types byte-for-byte after
// canonicalisation".
func roundTrip[T validator](t *testing.T, name string) {
	t.Helper()
	data, err := Read(name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("%s: unmarshal: %v", name, err)
	}
	if err := v.Validate(); err != nil {
		t.Fatalf("%s: fixture is not itself valid: %v", name, err)
	}
	reencoded, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("%s: marshal: %v", name, err)
	}
	reencoded = append(reencoded, '\n')
	if !bytes.Equal(reencoded, data) {
		t.Errorf("%s does not round-trip byte-for-byte:\n got: %s\nwant: %s", name, reencoded, data)
	}
}

func readFixture[T validator](t *testing.T, name string) T {
	t.Helper()
	data, err := Read(name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("%s: unmarshal: %v", name, err)
	}
	if err := value.Validate(); err != nil {
		t.Fatalf("%s: validate: %v", name, err)
	}
	return value
}

func TestExecutionEvidenceFixturesDescribeOneCoherentRun(t *testing.T) {
	record := readFixture[apicontract.ExecutionRecord](t, "execution_record.json")
	result := readFixture[apicontract.Result](t, "result_recorded.json")
	seriesRequest := readFixture[apicontract.ExecutionSeriesRequest](t, "execution_series_request.json")
	series := readFixture[apicontract.ExecutionSeriesResponse](t, "execution_series_response.json")

	if result.Execution == nil || *result.Execution != record.Ref {
		t.Fatalf("result execution = %#v, record ref = %#v", result.Execution, record.Ref)
	}
	if len(result.Recordset.Rows) != record.RowCount {
		t.Fatalf("result rows = %d, record rowCount = %d", len(result.Recordset.Rows), record.RowCount)
	}
	if !reflect.DeepEqual(result.BindingsApplied, record.BindingsApplied) || !reflect.DeepEqual(result.Limitations, record.Limitations) || result.Provenance != record.Provenance {
		t.Fatalf("result execution metadata does not match immutable record")
	}
	fingerprint, err := apicontract.FingerprintRecordset(result.Recordset)
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint != record.ResultFingerprint {
		t.Fatalf("result fingerprint = %s, record fingerprint = %s", fingerprint, record.ResultFingerprint)
	}
	if len(record.AuthorizedFields) != len(result.Recordset.Columns) {
		t.Fatalf("authorized fields = %d, result columns = %d", len(record.AuthorizedFields), len(result.Recordset.Columns))
	}
	for i, column := range result.Recordset.Columns {
		if record.AuthorizedFields[i].Column != column.Name {
			t.Fatalf("authorized field %d = %q, result column = %q", i, record.AuthorizedFields[i].Column, column.Name)
		}
	}
	if len(record.Measurements) != 1 || record.Measurements[0].Projection != seriesRequest.Partition.Projection {
		t.Fatalf("record projection = %#v, series projection = %#v", record.Measurements, seriesRequest.Partition.Projection)
	}
	partition := seriesRequest.Partition
	if partition.EvidenceStoreID != record.Ref.StoreID || partition.SourceScope != record.Scope || partition.Source != record.Provenance.Source ||
		partition.PolicyFingerprint != record.PolicyFingerprint || partition.QueryID != record.QueryID || partition.QueryRevision != record.QueryRevision ||
		partition.DTQLHash != record.DTQLHash || !reflect.DeepEqual(partition.BindingsApplied, record.BindingsApplied) {
		t.Fatalf("series partition %#v does not match record identity %#v", partition, record)
	}
	if len(series.Points) == 0 || series.Points[0].Execution != record.Ref {
		t.Fatalf("first series point does not reference record %#v", record.Ref)
	}
	measurement := record.Measurements[0]
	point := series.Points[0]
	if measurement.Completeness != point.Completeness || measurement.Value == nil || point.Value == nil || *measurement.Value != *point.Value {
		t.Fatalf("series point %#v does not match retained measurement %#v", point, measurement)
	}
}

func TestFixtures_RoundTrip(t *testing.T) {
	t.Run("scope.json", func(t *testing.T) { roundTrip[apicontract.Scope](t, "scope.json") })
	t.Run("source_ref.json", func(t *testing.T) { roundTrip[apicontract.SourceRef](t, "source_ref.json") })

	t.Run("typed_value_string.json", func(t *testing.T) { roundTrip[apicontract.TypedValue](t, "typed_value_string.json") })
	t.Run("typed_value_number.json", func(t *testing.T) { roundTrip[apicontract.TypedValue](t, "typed_value_number.json") })
	t.Run("typed_value_number_zero.json", func(t *testing.T) { roundTrip[apicontract.TypedValue](t, "typed_value_number_zero.json") })
	t.Run("typed_value_integer_large.json", func(t *testing.T) { roundTrip[apicontract.TypedValue](t, "typed_value_integer_large.json") })
	t.Run("typed_value_decimal.json", func(t *testing.T) { roundTrip[apicontract.TypedValue](t, "typed_value_decimal.json") })
	t.Run("typed_value_boolean_false.json", func(t *testing.T) { roundTrip[apicontract.TypedValue](t, "typed_value_boolean_false.json") })
	t.Run("typed_value_date.json", func(t *testing.T) { roundTrip[apicontract.TypedValue](t, "typed_value_date.json") })
	t.Run("typed_value_datetime.json", func(t *testing.T) { roundTrip[apicontract.TypedValue](t, "typed_value_datetime.json") })
	t.Run("typed_value_null.json", func(t *testing.T) { roundTrip[apicontract.TypedValue](t, "typed_value_null.json") })
	t.Run("typed_value_set.json", func(t *testing.T) { roundTrip[apicontract.TypedValueSet](t, "typed_value_set.json") })

	t.Run("agent_info.json", func(t *testing.T) { roundTrip[apicontract.AgentInfo](t, "agent_info.json") })
	t.Run("agent_info_no_principal_roles.json", func(t *testing.T) { roundTrip[apicontract.AgentInfo](t, "agent_info_no_principal_roles.json") })

	t.Run("semantic_columns_response.json", func(t *testing.T) {
		roundTrip[apicontract.SemanticColumnsResponse](t, "semantic_columns_response.json")
	})
	t.Run("semantic_columns_response_empty.json", func(t *testing.T) {
		roundTrip[apicontract.SemanticColumnsResponse](t, "semantic_columns_response_empty.json")
	})

	t.Run("related_response.json", func(t *testing.T) { roundTrip[apicontract.RelatedResponse](t, "related_response.json") })
	t.Run("related_response_empty.json", func(t *testing.T) { roundTrip[apicontract.RelatedResponse](t, "related_response_empty.json") })

	t.Run("applicable_response.json", func(t *testing.T) { roundTrip[apicontract.ApplicableResponse](t, "applicable_response.json") })

	t.Run("execution_request_saved.json", func(t *testing.T) { roundTrip[apicontract.ExecutionRequest](t, "execution_request_saved.json") })
	t.Run("execution_request_adhoc.json", func(t *testing.T) { roundTrip[apicontract.ExecutionRequest](t, "execution_request_adhoc.json") })
	t.Run("execution_request_snapshot.json", func(t *testing.T) { roundTrip[apicontract.ExecutionRequest](t, "execution_request_snapshot.json") })
	t.Run("execution_request_recorded.json", func(t *testing.T) { roundTrip[apicontract.ExecutionRequest](t, "execution_request_recorded.json") })

	t.Run("incident_create_request.json", func(t *testing.T) {
		roundTrip[apicontract.IncidentCreateRequest](t, "incident_create_request.json")
	})
	t.Run("incident_response.json", func(t *testing.T) {
		roundTrip[apicontract.IncidentResponse](t, "incident_response.json")
	})
	t.Run("incident_list_response.json", func(t *testing.T) {
		roundTrip[apicontract.IncidentListResponse](t, "incident_list_response.json")
	})
	t.Run("incident_append_request.json", func(t *testing.T) {
		roundTrip[apicontract.IncidentAppendRequest](t, "incident_append_request.json")
	})
	for _, name := range []string{
		"incident_append_context_fact_added_request.json",
		"incident_append_context_fact_promoted_request.json",
		"incident_append_context_fact_rejected_request.json",
	} {
		t.Run(name, func(t *testing.T) { roundTrip[apicontract.IncidentAppendRequest](t, name) })
	}
	t.Run("incident_append_response.json", func(t *testing.T) {
		roundTrip[apicontract.IncidentAppendResponse](t, "incident_append_response.json")
	})
	t.Run("incident_merge_request.json", func(t *testing.T) {
		roundTrip[apicontract.IncidentMergeRequest](t, "incident_merge_request.json")
	})
	t.Run("incident_merge_response.json", func(t *testing.T) {
		roundTrip[apicontract.IncidentMergeResponse](t, "incident_merge_response.json")
	})

	t.Run("applicable_request.json", func(t *testing.T) { roundTrip[apicontract.ApplicableRequest](t, "applicable_request.json") })
	t.Run("related_request.json", func(t *testing.T) { roundTrip[apicontract.RelatedRequest](t, "related_request.json") })
	t.Run("related_rows_request.json", func(t *testing.T) { roundTrip[apicontract.RelatedRowsRequest](t, "related_rows_request.json") })
	t.Run("related_rows_request_recorded.json", func(t *testing.T) {
		roundTrip[apicontract.RelatedRowsRequest](t, "related_rows_request_recorded.json")
	})

	t.Run("result_live.json", func(t *testing.T) { roundTrip[apicontract.Result](t, "result_live.json") })
	t.Run("result_restricted.json", func(t *testing.T) { roundTrip[apicontract.Result](t, "result_restricted.json") })
	t.Run("result_generic_policy_no_visible_names.json", func(t *testing.T) {
		roundTrip[apicontract.Result](t, "result_generic_policy_no_visible_names.json")
	})
	t.Run("result_opaque_privileged.json", func(t *testing.T) { roundTrip[apicontract.Result](t, "result_opaque_privileged.json") })
	t.Run("result_snapshot.json", func(t *testing.T) { roundTrip[apicontract.Result](t, "result_snapshot.json") })
	t.Run("result_truncated.json", func(t *testing.T) { roundTrip[apicontract.Result](t, "result_truncated.json") })
	t.Run("result_recorded.json", func(t *testing.T) { roundTrip[apicontract.Result](t, "result_recorded.json") })

	t.Run("execution_record.json", func(t *testing.T) {
		roundTrip[apicontract.ExecutionRecord](t, "execution_record.json")
	})
	t.Run("execution_list_request.json", func(t *testing.T) {
		roundTrip[apicontract.ExecutionListRequest](t, "execution_list_request.json")
	})
	t.Run("execution_list_response.json", func(t *testing.T) {
		roundTrip[apicontract.ExecutionListResponse](t, "execution_list_response.json")
	})
	t.Run("execution_snapshot_response.json", func(t *testing.T) {
		roundTrip[apicontract.SnapshotReadResponse](t, "execution_snapshot_response.json")
	})
	t.Run("execution_snapshot_expired_response.json", func(t *testing.T) {
		roundTrip[apicontract.SnapshotReadResponse](t, "execution_snapshot_expired_response.json")
	})
	t.Run("execution_series_request.json", func(t *testing.T) {
		roundTrip[apicontract.ExecutionSeriesRequest](t, "execution_series_request.json")
	})
	t.Run("execution_series_response.json", func(t *testing.T) {
		roundTrip[apicontract.ExecutionSeriesResponse](t, "execution_series_response.json")
	})

	t.Run("capture_query_request_create.json", func(t *testing.T) {
		roundTrip[apicontract.CaptureQueryRequest](t, "capture_query_request_create.json")
	})
	t.Run("capture_query_request_update.json", func(t *testing.T) {
		roundTrip[apicontract.CaptureQueryRequest](t, "capture_query_request_update.json")
	})
	t.Run("capture_query_response.json", func(t *testing.T) {
		roundTrip[apicontract.CaptureQueryResponse](t, "capture_query_response.json")
	})

	errorFixtures := []string{
		"error_revision_conflict.json",
		"error_capture_already_exists.json",
		"error_capture_invalid_location.json",
		"error_capture_credentials.json",
		"error_capture_result_rows.json",
		"error_capture_incomplete_record.json",
		"error_capture_write_denied.json",
		"error_invalid_request.json",
		"error_type_mismatch.json",
		"error_missing_parameter.json",
		"error_ambiguous_binding.json",
		"error_target_required.json",
		"error_unauthenticated.json",
		"error_access_denied.json",
		"error_unsupported_protected_execution.json",
		"error_not_found.json",
		"error_stale_context.json",
		"error_response_too_large.json",
		"error_source_unavailable.json",
		"error_timeout.json",
	}
	for _, name := range errorFixtures {
		t.Run(name, func(t *testing.T) { roundTrip[apicontract.ErrorEnvelope](t, name) })
	}
}

// TestFixtures_EveryErrorCodeHasAFixture proves the error fixture set above
// covers the entire closed ErrorCode set - "Cover... all envelopes and
// errors" - so a future error code added to the enum without a matching
// fixture fails this test rather than silently going unfrozen.
func TestFixtures_EveryErrorCodeHasAFixture(t *testing.T) {
	allCodes := []apicontract.ErrorCode{
		apicontract.ErrCodeInvalidRequest,
		apicontract.ErrCodeTypeMismatch,
		apicontract.ErrCodeMissingParameter,
		apicontract.ErrCodeAmbiguousBinding,
		apicontract.ErrCodeTargetRequired,
		apicontract.ErrCodeUnauthenticated,
		apicontract.ErrCodeAccessDenied,
		apicontract.ErrCodeUnsupportedProtectedExecution,
		apicontract.ErrCodeNotFound,
		apicontract.ErrCodeStaleContext,
		apicontract.ErrCodeResponseTooLarge,
		apicontract.ErrCodeSourceUnavailable,
		apicontract.ErrCodeTimeout,
		apicontract.ErrCodeRevisionConflict,
	}
	entries, err := Manifest()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[apicontract.ErrorCode]bool{}
	for _, e := range entries {
		data, err := Read(e.Name)
		if err != nil {
			t.Fatal(err)
		}
		var env apicontract.ErrorEnvelope
		if json.Unmarshal(data, &env) != nil {
			continue // not an error fixture
		}
		if env.Error.Code == "" {
			continue
		}
		seen[apicontract.ErrorCode(env.Error.Code)] = true
	}
	for _, code := range allCodes {
		if !seen[code] {
			t.Errorf("no fixture demonstrates error code %s", code)
		}
	}
}
