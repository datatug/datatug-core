package walkthrough

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegerStringAndKnownKinds(t *testing.T) {
	require.Equal(t, "42", Integer("42").String())

	for _, kind := range []string{
		"table-record",
		"query",
		"board",
		"card",
		"related",
		"log-source",
		"public-data-provider",
		"incident",
		"check",
		"none",
	} {
		require.True(t, KnownTargetKind(kind), kind)
	}
	require.False(t, KnownTargetKind("future-target"))

	for _, kind := range []string{
		"route-reached",
		"parameter-value",
		"record-selected",
		"query-run",
		"panel-open",
		"filter-applied",
		"check-run",
		"incident-open",
		"asset-exists",
		"board-tutorial",
		"acknowledged",
	} {
		require.True(t, KnownConditionKind(kind), kind)
	}
	require.False(t, KnownConditionKind("future-condition"))
}

func TestErrorAccessors(t *testing.T) {
	result := ParseDeclaration("tour", []byte(`{"x":tru}`))
	require.False(t, result.Accepted)
	require.Error(t, result.Err())

	var refusal *Error
	require.ErrorAs(t, result.Err(), &refusal)
	require.True(t, refusal.Recoverable())
	require.Equal(t, CodeInvalidJSON, ErrorCode(result.Err()))
	require.Contains(t, result.Err().Error(), `invalid_json at "/x"`)

	single := refusal.Issues[0]
	require.Contains(t, single.Error(), `invalid_json at "/x"`)

	multiple := &Error{Issues: []Issue{
		{Code: CodeMissingField, Path: "/first", Message: "first issue"},
		{Code: CodeInvalidType, Path: "/second", Message: "second issue"},
	}}
	require.Contains(t, multiple.Error(), "2 declaration issues")
	require.Contains(t, multiple.Error(), `missing_field at "/first"`)
	require.True(t, multiple.Recoverable())

	require.Equal(t, Code(""), ErrorCode(&Error{}))
	require.Equal(t, CodeTooLarge, ErrorCode(Issue{Code: CodeTooLarge, Message: "too large"}))
	require.Equal(t, Code(""), ErrorCode(errors.New("unrelated")))
	require.Equal(t, Code(""), ErrorCode(nil))
}

func TestBooleanScalarProjection(t *testing.T) {
	result := ParseDeclaration("tour", []byte(declarationWithStepBody(
		`"target":`+validTargetJSON+`,"done":{"kind":"parameter-value","name":"flag","equals":true}`,
	)))
	require.True(t, result.Accepted, "%+v", result.Issues)

	condition := result.Definition.Chapters[0].Steps[0].Done
	require.NotNil(t, condition.ParameterValue)
	require.NotNil(t, condition.ParameterValue.Equals)
	require.Equal(t, ScalarBoolean, condition.ParameterValue.Equals.Kind)
	require.True(t, condition.ParameterValue.Equals.Bool)

	projected := result.Definition.Projection()
	chapters := projected["chapters"].([]any)
	steps := chapters[0].(map[string]any)["steps"].([]any)
	done := steps[0].(map[string]any)["done"].(map[string]any)
	require.Equal(t, map[string]any{"kind": "boolean", "value": true}, done["equals"])
}

func TestProjectionOmitsUnknownScalarKind(t *testing.T) {
	// A caller can construct the exported model directly. An unrecognized
	// scalar kind is not one of the four declared projections, so it is
	// omitted rather than reported as a known scalar.
	declaration := &Declaration{
		ID:      "tour",
		Version: Integer("1"),
		Finish: Exit{Kind: "target", Target: &Target{
			Kind:      "incident",
			Supported: true,
			Incident: &IncidentTarget{Incident: &Value{
				Literal: &Scalar{Kind: ScalarKind("future-scalar")},
			}},
		}},
	}

	projected := declaration.Projection()
	finish := projected["finish"].(map[string]any)
	target := finish["target"].(map[string]any)
	incident := target["incident"].(map[string]any)
	require.Nil(t, incident["value"])
}
