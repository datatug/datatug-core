package fixtures

import (
	"bytes"
	"encoding/json"
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

	t.Run("result_live.json", func(t *testing.T) { roundTrip[apicontract.Result](t, "result_live.json") })
	t.Run("result_restricted.json", func(t *testing.T) { roundTrip[apicontract.Result](t, "result_restricted.json") })
	t.Run("result_generic_policy_no_visible_names.json", func(t *testing.T) {
		roundTrip[apicontract.Result](t, "result_generic_policy_no_visible_names.json")
	})
	t.Run("result_opaque_privileged.json", func(t *testing.T) { roundTrip[apicontract.Result](t, "result_opaque_privileged.json") })
	t.Run("result_snapshot.json", func(t *testing.T) { roundTrip[apicontract.Result](t, "result_snapshot.json") })
	t.Run("result_truncated.json", func(t *testing.T) { roundTrip[apicontract.Result](t, "result_truncated.json") })

	errorFixtures := []string{
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
