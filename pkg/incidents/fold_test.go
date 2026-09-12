package incidents

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFoldUsesVisibilityTimeAndIsByteDeterministic(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	createdAt := mustTime(t, "2026-09-12T10:00:00Z")
	mergeAt := mustTime(t, "2026-09-12T11:00:00Z")
	oldOccurrence := mustTime(t, "2026-09-11T09:00:00Z")
	events := []Event{
		createdEvent(t, ref, createdAt),
		{
			ID: "evt-imported", Seq: 2, At: oldOccurrence, VisibleAt: mergeAt,
			Incident: ref, Actor: Actor{Kind: ActorSystem, ID: "merge"},
			Type: EventNoteAdded, Assertion: Assertion{Kind: AssertionClaim},
			Refs:    []ArtifactRef{{Kind: RefIncident, Incident: &IncidentRef{StoreID: "ops", IncidentID: "INC-2"}}},
			Payload: mustJSON(t, NoteAddedPayload{Body: "older source event"}),
		},
	}

	beforeMerge := mustTime(t, "2026-09-12T10:30:00Z")
	before, err := Fold(events, &beforeMerge)
	require.NoError(t, err)
	require.Empty(t, before.Notes)

	after, err := Fold(events, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"older source event"}, after.Notes)

	first, err := json.Marshal(after)
	require.NoError(t, err)
	secondProjection, err := Fold(events, nil)
	require.NoError(t, err)
	second, err := json.Marshal(secondProjection)
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestFoldKeepsImportedEventsAsInertTimelineEvidence(t *testing.T) {
	destination := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	source := IncidentRef{StoreID: "ops", IncidentID: "INC-2"}
	createdAt := mustTime(t, "2026-09-12T10:00:00Z")
	mergeAt := mustTime(t, "2026-09-12T11:00:00Z")
	events := []Event{
		createdEvent(t, destination, createdAt),
		{
			ID: "merge-1-import-1", Seq: 2, At: createdAt.Add(-time.Hour), VisibleAt: mergeAt,
			Incident:     destination,
			ImportedFrom: &ImportedEventRef{Incident: source, EventID: "source-created", Seq: 1, MergeID: "merge-1"},
			Actor:        Actor{Kind: ActorHuman, ID: "source-owner"}, Type: EventIncidentCreated,
			Assertion: Assertion{Kind: AssertionClaim},
			Payload:   mustJSON(t, CreatedPayload{UID: "e665a6dd-a6f1-4c07-9931-3bb61399be12", Title: "Source incident"}),
		},
		{
			ID: "merge-1-import-2", Seq: 3, At: createdAt.Add(-30 * time.Minute), VisibleAt: mergeAt,
			Incident:     destination,
			ImportedFrom: &ImportedEventRef{Incident: source, EventID: "source-note", Seq: 2, MergeID: "merge-1"},
			Actor:        Actor{Kind: ActorHuman, ID: "source-owner"}, Type: EventNoteAdded,
			Assertion: Assertion{Kind: AssertionClaim},
			Payload:   mustJSON(t, NoteAddedPayload{Body: "source-only note"}),
		},
	}

	beforeMerge := mergeAt.Add(-time.Nanosecond)
	before, err := Fold(events, &beforeMerge)
	require.NoError(t, err)
	require.Equal(t, uint64(1), before.LastSeq)
	require.Equal(t, "Invoices stuck", before.Title)
	require.Empty(t, before.Notes)

	after, err := Fold(events, nil)
	require.NoError(t, err)
	require.Equal(t, uint64(3), after.LastSeq)
	require.Equal(t, "Invoices stuck", after.Title)
	require.Empty(t, after.Notes)

	atMerge, err := Fold(events, &mergeAt)
	require.NoError(t, err)
	require.Equal(t, after, atMerge)

	splitVisibility := append([]Event(nil), events...)
	splitVisibility[2].VisibleAt = mergeAt.Add(time.Second)
	_, err = Fold(splitVisibility, nil)
	require.ErrorContains(t, err, "split visibility")

	interleaved := append([]Event(nil), events...)
	interleaved[2] = eventWithPayload(t, destination, 3, mergeAt, EventNoteAdded, NoteAddedPayload{Body: "local note"})
	interleaved = append(interleaved, events[2])
	interleaved[3].Seq = 4
	_, err = Fold(interleaved, nil)
	require.ErrorContains(t, err, "not contiguous")

	mixedSource := append([]Event(nil), events...)
	mixedSource[2].ImportedFrom = &ImportedEventRef{
		Incident: IncidentRef{StoreID: "ops", IncidentID: "INC-3"}, EventID: "third-note", Seq: 2, MergeID: "merge-1",
	}
	_, err = Fold(mixedSource, nil)
	require.ErrorContains(t, err, "mixes source incidents")

	reordered := append([]Event(nil), events...)
	reordered[2].ImportedFrom = &ImportedEventRef{Incident: source, EventID: "source-note", Seq: 3, MergeID: "merge-1"}
	_, err = Fold(reordered, nil)
	require.ErrorContains(t, err, "source sequence is not contiguous")
}

func TestFoldValidatesInferenceAndLifecycle(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	at := mustTime(t, "2026-09-12T10:00:00Z")
	base := createdEvent(t, ref, at)

	t.Run("inference needs refs", func(t *testing.T) {
		inference := Event{
			ID: "evt-2", Seq: 2, At: at.Add(time.Minute), VisibleAt: at.Add(time.Minute),
			Incident: ref, Actor: Actor{Kind: ActorAgent, ID: "agent-1"},
			Type: EventNoteAdded, Assertion: Assertion{Kind: AssertionInference},
			Payload: mustJSON(t, NoteAddedPayload{Body: "maybe database contention"}),
		}
		_, err := Fold([]Event{base, inference}, nil)
		require.ErrorContains(t, err, "inference")
	})

	t.Run("inference needs an event ref", func(t *testing.T) {
		inference := Event{
			ID: "evt-2", Seq: 2, At: at.Add(time.Minute), VisibleAt: at.Add(time.Minute),
			Incident: ref, Actor: Actor{Kind: ActorAgent, ID: "agent-1"},
			Type: EventNoteAdded, Assertion: Assertion{Kind: AssertionInference},
			Refs:    []ArtifactRef{{Kind: RefExecution, Execution: &ExecutionRef{StoreID: "ops", ProjectID: "billing", ExecutionID: "exec-1"}}},
			Payload: mustJSON(t, NoteAddedPayload{Body: "maybe database contention"}),
		}
		require.ErrorContains(t, inference.Validate(), "event reference")
		inference.Refs = append(inference.Refs, ArtifactRef{Kind: RefEvent, ID: "evt-1"})
		require.NoError(t, inference.Validate())
	})

	t.Run("outcome only with resolved", func(t *testing.T) {
		outcome := Event{
			ID: "evt-2", Seq: 2, At: at.Add(time.Minute), VisibleAt: at.Add(time.Minute),
			Incident: ref, Actor: Actor{Kind: ActorHuman, ID: "alex"},
			Type: EventIncidentOutcome, Assertion: Assertion{Kind: AssertionClaim},
			Payload: mustJSON(t, OutcomePayload{Outcome: OutcomeResolved}),
		}
		_, err := Fold([]Event{base, outcome}, nil)
		require.ErrorContains(t, err, "resolved")
	})

	t.Run("valid resolution", func(t *testing.T) {
		status := eventWithPayload(t, ref, 2, at.Add(time.Minute), EventIncidentStatus, StatusPayload{Status: StatusResolved})
		outcome := eventWithPayload(t, ref, 3, at.Add(2*time.Minute), EventIncidentOutcome, OutcomePayload{Outcome: OutcomeResolved})
		projection, err := Fold([]Event{base, status, outcome}, nil)
		require.NoError(t, err)
		require.Equal(t, StatusResolved, projection.Status)
		require.Equal(t, OutcomeResolved, projection.Outcome)
	})
}

func createdEvent(t *testing.T, ref IncidentRef, at time.Time) Event {
	t.Helper()
	return Event{
		ID: "evt-1", Seq: 1, At: at, VisibleAt: at, Incident: ref,
		Actor: Actor{Kind: ActorHuman, ID: "alex"}, Type: EventIncidentCreated,
		Assertion: Assertion{Kind: AssertionClaim},
		Payload:   mustJSON(t, CreatedPayload{UID: "6ddfe079-d5fe-4d77-b322-11b9a31a9766", Title: "Invoices stuck", Description: "Billing queue is not draining"}),
	}
}

func eventWithPayload(t *testing.T, ref IncidentRef, seq uint64, at time.Time, eventType EventType, payload any) Event {
	t.Helper()
	return Event{ID: "evt-" + string(rune('0'+seq)), Seq: seq, At: at, VisibleAt: at, Incident: ref,
		Actor: Actor{Kind: ActorHuman, ID: "alex"}, Type: eventType,
		Assertion: Assertion{Kind: AssertionClaim}, Payload: mustJSON(t, payload)}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return parsed
}
