package incidents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func compareRunEvent(t *testing.T, ref IncidentRef, seq uint64, at time.Time) Event {
	t.Helper()
	comparison := ComparisonRef{
		Left:  ExecutionRef{StoreID: "evidence", ProjectID: "orders", ExecutionID: "left-1"},
		Right: ExecutionRef{StoreID: "evidence", ProjectID: "orders", ExecutionID: "right-1"},
	}
	return Event{
		ID: "compare-1", Seq: seq, At: at, VisibleAt: at, Incident: ref,
		Actor: Actor{Kind: ActorAgent, ID: "datatug"}, Type: EventCompareRun,
		Assertion: Assertion{Kind: AssertionDeterministicResult},
		Refs:      []ArtifactRef{{Kind: RefCompare, Comparison: &comparison}},
		Payload:   mustJSON(t, CompareRunPayload{Key: []string{"tenant_id", "order_id"}}),
	}
}

func TestCompareRunValidationAndProjection(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-8"}
	at := mustTime(t, "2026-09-14T10:00:00Z")
	event := compareRunEvent(t, ref, 2, at.Add(time.Minute))
	require.NoError(t, event.Validate())

	projection, err := Fold([]Event{createdEvent(t, ref, at), event}, nil)
	require.NoError(t, err)
	require.Equal(t, event.Refs, projection.AssetRefs)
	require.Equal(t, []AssetRefEntry{{EventID: event.ID, Ref: event.Refs[0]}}, projection.AssetRefEntries)
	replayed, err := Fold([]Event{createdEvent(t, ref, at), event}, nil)
	require.NoError(t, err)
	require.Equal(t, projection, replayed)
}

func TestCompareRunRejectsInvalidPayloadReferenceAndAssertion(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-8"}
	at := mustTime(t, "2026-09-14T10:00:00Z")
	base := compareRunEvent(t, ref, 2, at)

	wrongAssertion := base
	wrongAssertion.Assertion.Kind = AssertionObservation
	require.ErrorContains(t, wrongAssertion.Validate(), "deterministic-result")

	noReference := base
	noReference.Refs = nil
	require.ErrorContains(t, noReference.Validate(), "exactly one compare reference")

	twoReferences := base
	twoReferences.Refs = append(twoReferences.Refs, twoReferences.Refs[0])
	require.ErrorContains(t, twoReferences.Validate(), "exactly one compare reference")

	rowPayload := base
	rowPayload.Payload = []byte(`{"key":["id"],"rows":[{"secret":"value"}]}`)
	require.ErrorContains(t, rowPayload.Validate(), "unknown field")

	emptyKey := base
	emptyKey.Payload = mustJSON(t, CompareRunPayload{})
	require.ErrorContains(t, emptyKey.Validate(), "key is required")
}
