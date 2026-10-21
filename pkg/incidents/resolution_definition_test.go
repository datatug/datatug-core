package incidents

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolutionDefinitionDigestValidationAndLegacy(t *testing.T) {
	c, p, at := resolutionFixture()
	require.NoError(t, ValidateResolutionProof(c, p, at)) // Both absent: legacy contract.
	for _, value := range []string{"a", strings.Repeat("a", 63), strings.Repeat("A", 64), strings.Repeat("g", 64), " " + strings.Repeat("a", 64), strings.Repeat("a", 64) + " "} {
		t.Run(value, func(t *testing.T) {
			bad := c
			bad.DefinitionDigest = value
			require.ErrorContains(t, bad.Validate(), "lowercase SHA-256")
			badProof := p
			badProof.DefinitionDigest = value
			require.ErrorContains(t, badProof.Validate(at), "lowercase SHA-256")
		})
	}
	c.DefinitionDigest = strings.Repeat("a", 64)
	require.ErrorContains(t, ValidateResolutionProof(c, p, at), "digest mismatch")
	p.DefinitionDigest = strings.Repeat("b", 64)
	require.ErrorContains(t, ValidateResolutionProof(c, p, at), "digest mismatch")
	c.DefinitionDigest = ""
	require.ErrorContains(t, ValidateResolutionProof(c, p, at), "digest mismatch")
	c.Finalized = false
	c.FinalizedAt = nil
	c.DefinitionDigest = p.DefinitionDigest
	require.NoError(t, c.Validate()) // Supplied draft pin has a real check identity/revision.
	c.CheckID = ""
	c.CheckVersion = 0
	require.ErrorContains(t, c.Validate(), "requires check identity")
	c.DefinitionDigest = ""
	require.NoError(t, c.Validate()) // Incomplete legacy draft remains allowed.
}

func TestResolutionDefinitionDigestEventRoundTripAndImmutablePin(t *testing.T) {
	c, p, at := resolutionFixture()
	c.Version = 1
	p.CriterionVersion = 1
	c.DefinitionDigest = strings.Repeat("a", 64)
	p.DefinitionDigest = c.DefinitionDigest
	ref := IncidentRef{StoreID: "ops", IncidentID: "definition-pin"}
	events := []Event{createdEvent(t, ref, at.Add(-2*time.Minute)), eventWithPayload(t, ref, 2, *c.FinalizedAt, EventIncidentCriterionSet, c)}
	changed := c
	changed.Version++
	changed.FinalizedAt = &at
	changed.DefinitionDigest = strings.Repeat("b", 64)
	_, err := Fold(append(append([]Event(nil), events...), eventWithPayload(t, ref, 3, at, EventIncidentCriterionSet, changed)), nil)
	require.ErrorContains(t, err, "immutable")
	resolved := eventWithPayload(t, ref, 3, at, EventIncidentResolved, ResolvedPayload{HumanNote: "Carrier assignment check passed after replay", Proof: p})
	resolved.Refs = append([]ArtifactRef{{Kind: RefCheckRun, CheckRun: &p.Run}}, p.ObservationRefs...)
	resolved.Refs = append(resolved.Refs, p.RecoveryRefs...)
	events = append(events, resolved)
	raw, err := json.Marshal(events)
	require.NoError(t, err)
	var restored []Event
	require.NoError(t, json.Unmarshal(raw, &restored))
	projection, err := Fold(restored, nil)
	require.NoError(t, err)
	require.Equal(t, StatusResolved, projection.Status)
	require.Equal(t, c.DefinitionDigest, projection.ResolutionCriterion.DefinitionDigest)
	require.Equal(t, p.DefinitionDigest, projection.Resolution.Proof.DefinitionDigest)
	require.Equal(t, []string{"Carrier assignment check passed after replay"}, projection.Notes)
	legacy, _, _ := resolutionFixture()
	raw, err = json.Marshal(legacy)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "definitionDigest")
}
