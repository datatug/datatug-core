package apicontract

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	contractfixtures "github.com/datatug/datatug-core/pkg/apicontract/fixtures"
	"github.com/datatug/datatug-core/pkg/incidents"
	"github.com/datatug/datatug-core/pkg/investigation"
	"github.com/stretchr/testify/require"
)

func TestIncidentContextEventTransportStrictDecodeAndCompatibility(t *testing.T) {
	scope := investigation.ProjectScope{StoreID: "projects", ProjectID: "billing", Environment: "prod"}
	fact := investigation.Fact{
		ID: "customer-11", Entity: "Customer", Field: "ID", Value: NewIntegerValue("11"),
		Origin: FactOriginContext, Enabled: true, Role: FactRoleSuspected, Layer: "hypothesis:H17", Scope: &scope,
	}
	request := validAppendRequest()
	request.Event.Type = incidents.EventContextFactAdded
	request.Event.Payload = mustMarshalAPI(t, incidents.ContextFactAddedPayload{Fact: fact})
	require.NoError(t, request.Validate())

	wire, err := json.Marshal(request)
	require.NoError(t, err)
	var decoded IncidentAppendRequest
	require.NoError(t, DecodeStrict(wire, &decoded))
	require.NoError(t, decoded.Validate())
	require.Equal(t, request.Event.Type, decoded.Event.Type)
	duplicateRole := strings.Replace(string(wire), `"role":"suspected"`, `"role":"suspected","role":"affected"`, 1)
	require.ErrorContains(t, DecodeStrict([]byte(duplicateRole), &decoded), "duplicate key")

	caseVariantPayloads := []struct {
		name      string
		eventType incidents.EventType
		payload   json.RawMessage
	}{
		{
			name:      "added fact role",
			eventType: incidents.EventContextFactAdded,
			payload:   json.RawMessage(`{"fact":{"id":"customer-11","entity":"Customer","field":"ID","value":{"type":"integer","value":"11"},"origin":"context","enabled":true,"role":"suspected","Role":"affected","layer":"hypothesis:H17","scope":{"storeId":"projects","projectId":"billing","environment":"prod"}}}`),
		},
		{
			name:      "promoted role",
			eventType: incidents.EventContextFactPromoted,
			payload:   json.RawMessage(`{"fact":{"scope":{"storeId":"projects","projectId":"billing","environment":"prod"},"id":"customer-11","layer":"hypothesis:H17"},"role":"affected","Role":"excluded"}`),
		},
		{
			name:      "rejected layer",
			eventType: incidents.EventContextFactRejected,
			payload:   json.RawMessage(`{"layer":"hypothesis:H17","Layer":"hypothesis:H12"}`),
		},
	}
	for _, tt := range caseVariantPayloads {
		t.Run(tt.name, func(t *testing.T) {
			candidate := validAppendRequest()
			candidate.Event.Type = tt.eventType
			candidate.Event.Payload = tt.payload
			require.ErrorContains(t, candidate.Validate(), "duplicate key")

			candidateWire, err := json.Marshal(candidate)
			require.NoError(t, err)
			var strict IncidentAppendRequest
			require.ErrorContains(t, DecodeStrict(candidateWire, &strict), "duplicate key")
		})
	}

	request.Event.Payload = json.RawMessage(`{"fact":{"id":"customer-11","entity":"Customer","field":"ID","value":{"type":"integer","value":"11"},"origin":"context","enabled":true,"role":"suspected","layer":"hypothesis:H17","scope":{"storeId":"projects","projectId":"billing","environment":"prod"}},"unexpected":true}`)
	require.ErrorContains(t, request.Validate(), "unknown field")

	legacy := validAppendRequest()
	legacy.Event.Type = incidents.EventNoteAdded
	legacy.Event.Payload = mustMarshalAPI(t, incidents.NoteAddedPayload{Body: "legacy note"})
	require.NoError(t, legacy.Validate())
}

func TestIncidentContextEventFixturesStrictDecode(t *testing.T) {
	for _, name := range []string{
		"incident_append_context_fact_added_request.json",
		"incident_append_context_fact_promoted_request.json",
		"incident_append_context_fact_rejected_request.json",
	} {
		t.Run(name, func(t *testing.T) {
			wire, err := contractfixtures.Read(name)
			require.NoError(t, err)
			var request IncidentAppendRequest
			require.NoError(t, DecodeStrict(wire, &request))
			require.NoError(t, request.Validate())
		})
	}
}

func TestIncidentPromotionTransportDoesNotInventHypothesisRefRequirement(t *testing.T) {
	require.Equal(t, "canonical", FactLayerCanonical)
	scope := investigation.ProjectScope{StoreID: "projects", ProjectID: "billing", Environment: "prod"}
	request := validAppendRequest()
	request.Event.At = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	request.Event.Type = incidents.EventContextFactPromoted
	request.Event.Payload = mustMarshalAPI(t, incidents.ContextFactPromotedPayload{
		Fact: incidents.ContextFactRef{Scope: scope, ID: "customer-11", Layer: "hypothesis:H17"},
		Role: investigation.FactRoleAffected,
	})
	require.Empty(t, request.Event.Refs)
	require.NoError(t, request.Validate())
}

func mustMarshalAPI(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}
