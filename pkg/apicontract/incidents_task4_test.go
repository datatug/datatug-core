package apicontract

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

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

	request.Event.Payload = json.RawMessage(`{"fact":{"id":"customer-11","entity":"Customer","field":"ID","value":{"type":"integer","value":"11"},"origin":"context","enabled":true,"role":"suspected","layer":"hypothesis:H17","scope":{"storeId":"projects","projectId":"billing","environment":"prod"}},"unexpected":true}`)
	require.ErrorContains(t, request.Validate(), "unknown field")

	legacy := validAppendRequest()
	legacy.Event.Type = incidents.EventNoteAdded
	legacy.Event.Payload = mustMarshalAPI(t, incidents.NoteAddedPayload{Body: "legacy note"})
	require.NoError(t, legacy.Validate())
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
