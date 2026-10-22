package incidents

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func operationFixture() ArtifactRef {
	return ArtifactRef{Kind: RefSourceOperation, SourceOperation: &SourceOperationRef{StoreID: "firestore", SpaceID: "space", ProjectID: "project", SourceID: "source", ReceiptActorID: "actor", OperationID: "op", RequestSHA256: strings.Repeat("a", 64), Result: SourceOperationResult{Generation: "g-1", Revision: 2, SHA256: strings.Repeat("b", 64)}}}
}
func TestSourceOperationContractRoundTripAndValidation(t *testing.T) {
	ref := operationFixture()
	require.NoError(t, ref.Validate())
	encoded, err := json.Marshal(ref)
	require.NoError(t, err)
	var decoded ArtifactRef
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, ref, decoded)
	require.Contains(t, string(encoded), `"receiptActorId":"actor"`)
	decoder := json.NewDecoder(bytes.NewReader([]byte(strings.Replace(string(encoded), `"sourceOperation":{`, `"sourceOperation":{"environment":"demo",`, 1))))
	decoder.DisallowUnknownFields()
	require.Error(t, decoder.Decode(&decoded))
	for _, mutate := range []func(*ArtifactRef){
		func(r *ArtifactRef) { r.SourceOperation = nil; r.ID = "op" },
		func(r *ArtifactRef) { r.ID = "op" },
		func(r *ArtifactRef) { r.Kind = RefEvent },
		func(r *ArtifactRef) { r.SourceOperation.SpaceID = ".." },
		func(r *ArtifactRef) { r.SourceOperation.ReceiptActorID = strings.Repeat("x", 257) },
		func(r *ArtifactRef) { r.SourceOperation.Result.Revision = 0 },
		func(r *ArtifactRef) { r.SourceOperation.RequestSHA256 = "bad" },
		func(r *ArtifactRef) { r.SourceOperation.Result.SHA256 = strings.Repeat("A", 64) },
	} {
		bad := operationFixture()
		mutate(&bad)
		require.Error(t, bad.Validate())
	}
	missing := ArtifactRef{Kind: RefSourceOperation, Execution: &ExecutionRef{StoreID: "s", ProjectID: "p", ExecutionID: "e"}}
	require.Error(t, missing.Validate())
}
func TestSourceOperationFoldCopyEqualityAndVisibility(t *testing.T) {
	c, proof, at := resolutionFixture()
	c.Version = 1
	proof.CriterionVersion = 1
	proof.RecoveryRefs = []ArtifactRef{operationFixture()}
	ref := IncidentRef{StoreID: "ops", IncidentID: "incident-1"}
	events := []Event{createdEvent(t, ref, at.Add(-120000000000)), eventWithPayload(t, ref, 2, *c.FinalizedAt, EventIncidentCriterionSet, c)}
	resolved := eventWithPayload(t, ref, 3, at, EventIncidentResolved, ResolvedPayload{HumanNote: "Recovered", Proof: proof})
	resolved.Refs = append([]ArtifactRef{{Kind: RefCheckRun, CheckRun: &proof.Run}}, proof.ObservationRefs...)
	resolved.Refs = append(resolved.Refs, proof.RecoveryRefs...)
	projection, err := Fold(append(events, resolved), nil)
	require.NoError(t, err)
	view := ApplyIncidentView(projection, ViewPolicy{})
	require.Equal(t, proof.RecoveryRefs, view.Resolution.Proof.RecoveryRefs)
	view.Resolution.Proof.RecoveryRefs[0].SourceOperation.Result.Generation = "mutated"
	require.Equal(t, "g-1", projection.Resolution.Proof.RecoveryRefs[0].SourceOperation.Result.Generation)
	copy := cloneArtifactRef(proof.RecoveryRefs[0])
	require.True(t, artifactRefsEqual(copy, proof.RecoveryRefs[0]))
	copy.SourceOperation.OperationID = "other"
	require.False(t, artifactRefsEqual(copy, proof.RecoveryRefs[0]))
	require.False(t, hasEvidenceRef([]ArtifactRef{operationFixture()})) // receipt alone is no deterministic result
}
