package incidents

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func reopenFixture(t *testing.T) ([]Event, ResolutionProof, time.Time) {
	t.Helper()
	criterion, proof, at := resolutionFixture()
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	criterion.Version = 1
	proof.CriterionVersion = 1
	resolved := eventWithPayload(t, ref, 3, at, EventIncidentResolved, ResolvedPayload{HumanNote: "First resolution", Proof: proof})
	resolved.Refs = append([]ArtifactRef{{Kind: RefCheckRun, CheckRun: &proof.Run}}, proof.ObservationRefs...)
	resolved.Refs = append(resolved.Refs, proof.RecoveryRefs...)
	return []Event{createdEvent(t, ref, at.Add(-2*time.Minute)), eventWithPayload(t, ref, 2, *criterion.FinalizedAt, EventIncidentCriterionSet, criterion), resolved}, proof, at
}
func TestReopenRetainsHistoryAndRequiresFreshResolutionEpoch(t *testing.T) {
	events, proof, at := reopenFixture(t)
	ref := events[0].Incident
	before, err := Fold(events, nil)
	require.NoError(t, err)
	reopenAt := at.Add(time.Second)
	reopened := eventWithPayload(t, ref, 4, reopenAt, EventIncidentReopened, ReopenedPayload{Reason: "New evidence invalidates recovery", ExpectedResolutionEpoch: 0})
	events = append(events, reopened)
	current, err := Fold(events, nil)
	require.NoError(t, err)
	require.Equal(t, StatusInvestigating, current.Status)
	require.Equal(t, uint64(1), current.ResolutionEpoch)
	require.Equal(t, &reopenAt, current.ReopenedAt)
	require.Nil(t, current.Resolution)
	require.Empty(t, current.ResolutionEventID)
	require.Empty(t, current.Outcome)
	require.Equal(t, before.ResolutionCriterion, current.ResolutionCriterion)
	require.Equal(t, before.CriterionEventID, current.CriterionEventID)
	require.Equal(t, []string{"First resolution", "New evidence invalidates recovery"}, current.Notes)
	historical, err := Fold(events, &at)
	require.NoError(t, err)
	require.Equal(t, before, historical)
	require.ErrorContains(t, ValidateIncidentResolutionProof(current, proof, reopenAt.Add(time.Second)), "epoch")
	stale := proof
	stale.ResolutionEpoch = 1
	require.ErrorContains(t, ValidateIncidentResolutionProof(current, stale, reopenAt.Add(time.Second)), "after reopening")
	stale.EvaluatedAt = reopenAt
	require.ErrorContains(t, ValidateIncidentResolutionProof(current, stale, reopenAt.Add(time.Second)), "after reopening")
	fresh := proof
	fresh.ResolutionEpoch = 1
	fresh.Run.RunID = "new-passing-run"
	fresh.EvaluatedAt = reopenAt.Add(time.Second)
	resolved := eventWithPayload(t, ref, 5, reopenAt.Add(2*time.Second), EventIncidentResolved, ResolvedPayload{HumanNote: "Fresh evidence confirms recovery", Proof: fresh})
	resolved.Refs = append([]ArtifactRef{{Kind: RefCheckRun, CheckRun: &fresh.Run}}, fresh.ObservationRefs...)
	resolved.Refs = append(resolved.Refs, fresh.RecoveryRefs...)
	final, err := Fold(append(events, resolved), nil)
	require.NoError(t, err)
	require.Equal(t, StatusResolved, final.Status)
	require.Equal(t, "new-passing-run", final.Resolution.Proof.Run.RunID)
	require.NoError(t, final.Validate())
	view := ApplyIncidentView(final, ViewPolicy{})
	require.NoError(t, view.Validate())
	require.Equal(t, uint64(1), view.ResolutionEpoch)
	*view.ReopenedAt = view.ReopenedAt.Add(time.Hour)
	require.Equal(t, reopenAt, *final.ReopenedAt)
	// A stale expected epoch cannot reopen the subsequent resolution.
	staleReopen := eventWithPayload(t, ref, 6, reopenAt.Add(3*time.Second), EventIncidentReopened, ReopenedPayload{Reason: "Third attempt", ExpectedResolutionEpoch: 0})
	_, err = Fold(append(append(events, resolved), staleReopen), nil)
	require.ErrorContains(t, err, "epoch conflict")
	staleReopen.Payload = mustJSON(t, ReopenedPayload{Reason: "Third attempt", ExpectedResolutionEpoch: 1})
	next, err := Fold(append(append(events, resolved), staleReopen), nil)
	require.NoError(t, err)
	require.Equal(t, uint64(2), next.ResolutionEpoch)
	require.ErrorContains(t, ValidateIncidentResolutionProof(next, fresh, reopenAt.Add(4*time.Second)), "epoch")
}
func TestReopenRejectsUnsafeTransitionsAndBackdatedProof(t *testing.T) {
	events, proof, at := reopenFixture(t)
	ref := events[0].Incident
	for _, status := range []Status{StatusResolved, StatusWatching, StatusClosed} {
		t.Run(string(status), func(t *testing.T) {
			stream := append([]Event(nil), events...)
			if status != StatusResolved {
				stream = append(stream, eventWithPayload(t, ref, 4, at.Add(time.Second), EventIncidentOutcome, OutcomePayload{Outcome: OutcomeResolved}), eventWithPayload(t, ref, 5, at.Add(2*time.Second), EventIncidentStatus, StatusPayload{Status: status}))
			}
			reopen := eventWithPayload(t, ref, uint64(len(stream)+1), at.Add(3*time.Second), EventIncidentReopened, ReopenedPayload{Reason: "Investigate again"})
			reopen.At = at.Add(-time.Hour) // occurrence time is not the commit fence
			current, err := Fold(append(stream, reopen), nil)
			require.NoError(t, err)
			require.Equal(t, reopen.VisibleAt, *current.ReopenedAt)
			stale := proof
			stale.ResolutionEpoch = 1
			stale.EvaluatedAt = at.Add(time.Second)
			require.ErrorContains(t, ValidateIncidentResolutionProof(current, stale, at.Add(4*time.Second)), "after reopening")
		})
	}
	for _, status := range []Status{StatusOpen, StatusInvestigating, StatusMitigating, StatusRecovering} {
		t.Run(string(status), func(t *testing.T) {
			stream := []Event{events[0]}
			if status != StatusOpen {
				stream = append(stream, eventWithPayload(t, ref, 2, at, EventIncidentStatus, StatusPayload{Status: status}))
			}
			_, err := Fold(append(stream, eventWithPayload(t, ref, uint64(len(stream)+1), at.Add(time.Second), EventIncidentReopened, ReopenedPayload{Reason: "invalid"})), nil)
			require.ErrorContains(t, err, "only a resolved")
		})
	}
	_, err := Fold(append(events, eventWithPayload(t, ref, 4, at.Add(time.Second), EventIncidentStatus, StatusPayload{Status: StatusInvestigating})), nil)
	require.ErrorContains(t, err, "backward")
	merged := eventWithPayload(t, ref, 4, at.Add(time.Second), EventIncidentMerged, MergedPayload{Into: IncidentRef{StoreID: "ops", IncidentID: "INC-2"}, MergeID: "merge"})
	_, err = Fold(append(append(events, merged), eventWithPayload(t, ref, 5, at.Add(2*time.Second), EventIncidentReopened, ReopenedPayload{Reason: "invalid"})), nil)
	require.ErrorContains(t, err, "terminal")
}
func TestReopenPayloadEpochAndSequenceValidation(t *testing.T) {
	events, _, at := reopenFixture(t)
	ref := events[0].Incident
	event := eventWithPayload(t, ref, 4, at.Add(time.Second), EventIncidentReopened, ReopenedPayload{Reason: "Investigate"})
	for _, raw := range []string{`{"reason":""}`, `{"reason":"valid","unexpected":1}`, `{"reason":"first","reason":"second"}`, `{"reason":"valid","expectedResolutionEpoch":-1}`} {
		event.Payload = []byte(raw)
		require.Error(t, event.Validate())
	}
	require.Error(t, (ReopenedPayload{Reason: strings.Repeat("a", 4097)}).Validate())
	require.Error(t, (ReopenedPayload{Reason: "valid", ExpectedResolutionEpoch: math.MaxUint64}).Validate())
	event.Payload = mustJSON(t, ReopenedPayload{Reason: "Investigate"})
	mutation := Mutation{MutationID: "reopen", Incident: ref, Event: EventDraft{At: event.At, Actor: event.Actor, Type: event.Type, Assertion: event.Assertion, Payload: event.Payload}}
	require.ErrorContains(t, mutation.Validate(), "expectedSeq")
	zero := uint64(0)
	mutation.ExpectedSeq = &zero
	require.Error(t, mutation.Validate())
	seq := uint64(3)
	mutation.ExpectedSeq = &seq
	require.NoError(t, mutation.Validate())
	base, err := Fold(events, nil)
	require.NoError(t, err)
	invalid := base
	invalid.ReopenedAt = &at
	require.ErrorContains(t, invalid.Validate(), "legacy resolution epoch")
	invalid = base
	invalid.ResolutionEpoch = 1
	require.ErrorContains(t, invalid.Validate(), "reopening time")
	wrongZone := at.In(time.FixedZone("offset", 3600))
	invalid.ReopenedAt = &wrongZone
	require.ErrorContains(t, invalid.Validate(), "UTC")
	empty := Incident{}
	_, proof, now := resolutionFixture()
	require.ErrorContains(t, ValidateIncidentResolutionProof(empty, proof, now), "criterion")
}

func TestVersionedIncidentStatusAllowsReviewedActiveTransitions(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	at := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	stream := []Event{createdEvent(t, ref, at)}
	for _, status := range []Status{StatusMitigating, StatusInvestigating, StatusRecovering, StatusInvestigating, StatusRecovering, StatusResolved} {
		stream = append(stream, eventWithPayload(t, ref, uint64(len(stream)+1), at.Add(time.Duration(len(stream))*time.Second), EventIncidentStatus, StatusPayload{Status: status, LifecycleVersion: 1}))
	}
	current, err := Fold(stream, nil)
	require.NoError(t, err)
	require.Equal(t, StatusResolved, current.Status)
	_, err = Fold(append(stream, eventWithPayload(t, ref, uint64(len(stream)+1), at.Add(time.Minute), EventIncidentStatus, StatusPayload{Status: StatusInvestigating, LifecycleVersion: 1})), nil)
	require.ErrorContains(t, err, "illegal")
	for _, version := range []int{-1, 2} {
		event := eventWithPayload(t, ref, 2, at, EventIncidentStatus, StatusPayload{Status: StatusInvestigating, LifecycleVersion: version})
		require.Error(t, event.Validate())
	}
	require.False(t, CanTransitionIncidentStatus(StatusClosed, StatusInvestigating))
	require.False(t, CanTransitionIncidentStatus(StatusWatching, StatusInvestigating))
	require.True(t, CanTransitionIncidentStatus(StatusWatching, StatusClosed))
	require.True(t, CanTransitionIncidentStatus(StatusResolved, StatusWatching))
	require.False(t, CanTransitionIncidentStatus(StatusOpen, StatusResolved))
	require.False(t, CanTransitionIncidentStatus(StatusInvestigating, StatusInvestigating))
	// A historic forward skip remains valid; the same new Work request is rejected.
	_, err = Fold([]Event{stream[0], eventWithPayload(t, ref, 2, at.Add(time.Second), EventIncidentStatus, StatusPayload{Status: StatusResolved})}, nil)
	require.NoError(t, err)
	_, err = Fold([]Event{stream[0], eventWithPayload(t, ref, 2, at.Add(time.Second), EventIncidentStatus, StatusPayload{Status: StatusResolved, LifecycleVersion: 1})}, nil)
	require.ErrorContains(t, err, "illegal")
	// The new transition policy cannot bypass an evidence-required criterion.
	evidence, _, wall := reopenFixture(t)
	evidence = evidence[:2]
	evidence = append(evidence, eventWithPayload(t, evidence[0].Incident, 3, wall, EventIncidentStatus, StatusPayload{Status: StatusMitigating, LifecycleVersion: 1}), eventWithPayload(t, evidence[0].Incident, 4, wall.Add(time.Second), EventIncidentStatus, StatusPayload{Status: StatusRecovering, LifecycleVersion: 1}))
	_, err = Fold(append(evidence, eventWithPayload(t, evidence[0].Incident, 5, wall.Add(2*time.Second), EventIncidentStatus, StatusPayload{Status: StatusResolved, LifecycleVersion: 1})), nil)
	require.ErrorContains(t, err, "required resolution criterion")
}
