package incidents

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProjectAndExecutionRefsValidate(t *testing.T) {
	require.NoError(t, (ProjectRef{StoreID: "ops", ProjectID: "billing"}).Validate())
	require.Error(t, (ProjectRef{StoreID: "", ProjectID: "billing"}).Validate())
	require.Error(t, (ProjectRef{StoreID: "bad/store", ProjectID: "billing"}).Validate())
	require.Error(t, (ProjectRef{StoreID: "ops"}).Validate())
	require.Error(t, (ProjectRef{StoreID: " ops", ProjectID: "billing"}).Validate())
	require.Error(t, (ProjectRef{StoreID: "ops", ProjectID: " billing"}).Validate())

	require.NoError(t, (ExecutionRef{StoreID: "ops", ProjectID: "billing", ExecutionID: "exec-1"}).Validate())
	require.Error(t, (ExecutionRef{StoreID: "", ProjectID: "billing", ExecutionID: "exec-1"}).Validate())
	require.Error(t, (ExecutionRef{StoreID: "bad/store", ProjectID: "billing", ExecutionID: "exec-1"}).Validate())
	require.Error(t, (ExecutionRef{StoreID: "ops", ExecutionID: "exec-1"}).Validate())
	require.Error(t, (ExecutionRef{StoreID: "ops", ProjectID: " billing", ExecutionID: "exec-1"}).Validate())
	require.Error(t, (ExecutionRef{StoreID: "ops", ProjectID: "billing", ExecutionID: " exec-1"}).Validate())

	require.NoError(t, (ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", ID: "check-1"}).Validate())
	require.Error(t, (ProjectArtifactRef{StoreID: "ops", ProjectID: "billing"}).Validate())
	require.Error(t, (ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", ID: " check-1"}).Validate())
	require.Error(t, (ProjectArtifactRef{StoreID: "", ProjectID: "billing", ID: "check-1"}).Validate())
	require.NoError(t, (ComparisonRef{
		Left:  ExecutionRef{StoreID: "ops", ProjectID: "billing", ExecutionID: "exec-1"},
		Right: ExecutionRef{StoreID: "ops", ProjectID: "billing", ExecutionID: "exec-2"},
	}).Validate())
	require.Error(t, (ComparisonRef{Right: ExecutionRef{StoreID: "ops", ProjectID: "billing", ExecutionID: "exec-2"}}).Validate())
	require.Error(t, (ComparisonRef{Left: ExecutionRef{StoreID: "ops", ProjectID: "billing", ExecutionID: "exec-1"}}).Validate())
}

func TestArtifactRefValidation(t *testing.T) {
	incident := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	project := ProjectRef{StoreID: "ops", ProjectID: "billing"}
	execution := ExecutionRef{StoreID: "ops", ProjectID: "billing", ExecutionID: "exec-1"}
	artifact := ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", ID: "asset-1"}
	comparison := ComparisonRef{Left: execution, Right: ExecutionRef{StoreID: "ops", ProjectID: "billing", ExecutionID: "exec-2"}}

	valid := []ArtifactRef{
		{Kind: RefIncident, Incident: &incident},
		{Kind: RefProject, Project: &project},
		{Kind: RefExecution, Execution: &execution},
		{Kind: RefSnapshot, Execution: &execution},
		{Kind: RefEvent, ID: "evt-1"}, {Kind: RefHypothesis, ID: "H1"},
		{Kind: RefFact, Artifact: &artifact}, {Kind: RefAnnotation, Artifact: &artifact},
		{Kind: RefCheck, Artifact: &artifact}, {Kind: RefCompare, Comparison: &comparison},
		{Kind: RefQuery, Artifact: &artifact}, {Kind: RefBoard, Artifact: &artifact},
	}
	for _, ref := range valid {
		require.NoError(t, ref.Validate(), ref.Kind)
	}

	invalid := []ArtifactRef{
		{}, {Kind: "other", ID: "x"}, {Kind: RefIncident, ID: "INC-1"},
		{Kind: RefProject, ID: "billing"}, {Kind: RefExecution, ID: "exec-1"},
		{Kind: RefEvent}, {Kind: RefEvent, ID: "evt-1", Incident: &incident},
		{Kind: RefFact, ID: "fact-1"}, {Kind: RefCheck, Incident: &incident},
		{Kind: RefCompare, ID: "cmp-1"},
		{Kind: RefIncident, Incident: &IncidentRef{StoreID: "bad/store", IncidentID: "INC-1"}},
		{Kind: RefProject, Project: &ProjectRef{StoreID: "ops"}},
		{Kind: RefExecution, Execution: &ExecutionRef{StoreID: "ops"}},
	}
	for _, ref := range invalid {
		require.Error(t, ref.Validate(), ref.Kind)
	}
}

func TestEventValidation(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	at := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	valid := createdEvent(t, ref, at)
	require.NoError(t, valid.Validate())

	tests := []struct {
		name string
		edit func(*Event)
	}{
		{"missing id", func(e *Event) { e.ID = "" }},
		{"zero seq", func(e *Event) { e.Seq = 0 }},
		{"zero at", func(e *Event) { e.At = time.Time{} }},
		{"zero visibleAt", func(e *Event) { e.VisibleAt = time.Time{} }},
		{"bad incident", func(e *Event) { e.Incident.StoreID = "bad/store" }},
		{"bad actor kind", func(e *Event) { e.Actor.Kind = "robot" }},
		{"missing actor id", func(e *Event) { e.Actor.ID = "" }},
		{"bad actor via", func(e *Event) { e.Actor.Via = "email" }},
		{"bad assertion", func(e *Event) { e.Assertion.Kind = "fact" }},
		{"bad confidence", func(e *Event) { e.Assertion.Confidence = "certain" }},
		{"missing payload", func(e *Event) { e.Payload = nil }},
		{"bad ref", func(e *Event) { e.Refs = []ArtifactRef{{Kind: RefEvent}} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := valid
			test.edit(&event)
			require.Error(t, event.Validate())
		})
	}

	for _, assertion := range []AssertionKind{AssertionObservation, AssertionClaim, AssertionQuestion, AssertionHypothesis, AssertionDeterministicResult} {
		event := valid
		event.Assertion.Kind = assertion
		require.NoError(t, event.Validate(), assertion)
	}
	for _, via := range []string{"", ActorViaWeb, ActorViaCLI, ActorViaAPI, ActorViaSlack} {
		event := valid
		event.Actor.Via = via
		require.NoError(t, event.Validate(), via)
	}
	for _, confidence := range []string{"", ConfidenceSpeculative, ConfidenceLikely, ConfidenceConfirmed} {
		event := valid
		event.Assertion.Confidence = confidence
		require.NoError(t, event.Validate(), confidence)
	}

	agent := valid
	agent.Actor.Kind = ActorAgent
	agent.Assertion.Kind = AssertionObservation
	require.Error(t, agent.Validate())
	agent.Refs = []ArtifactRef{{Kind: RefExecution, Execution: &ExecutionRef{StoreID: "ops", ProjectID: "billing", ExecutionID: "exec-1"}}}
	require.NoError(t, agent.Validate())
}

func TestImportedEventValidation(t *testing.T) {
	destination := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	source := IncidentRef{StoreID: "ops", IncidentID: "INC-2"}
	at := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	valid := eventWithPayload(t, destination, 2, at, EventNoteAdded, NoteAddedPayload{Body: "source note"})
	valid.ImportedFrom = &ImportedEventRef{Incident: source, EventID: "source-event-1", Seq: 1, MergeID: "merge-1"}
	require.NoError(t, valid.Validate())

	tests := []struct {
		name string
		edit func(*Event)
	}{
		{"first event", func(e *Event) { e.Seq = 1 }},
		{"same incident", func(e *Event) { e.ImportedFrom.Incident = destination }},
		{"different store", func(e *Event) { e.ImportedFrom.Incident.StoreID = "other" }},
		{"invalid source incident", func(e *Event) { e.ImportedFrom.Incident.IncidentID = "bad/incident" }},
		{"missing source event id", func(e *Event) { e.ImportedFrom.EventID = "" }},
		{"zero source sequence", func(e *Event) { e.ImportedFrom.Seq = 0 }},
		{"missing merge id", func(e *Event) { e.ImportedFrom.MergeID = "" }},
		{"invalid merge id", func(e *Event) { e.ImportedFrom.MergeID = "../merge" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := valid
			importedFrom := *valid.ImportedFrom
			event.ImportedFrom = &importedFrom
			test.edit(&event)
			require.Error(t, event.Validate())
		})
	}
}

func TestEventValidationRejectsInvalidTypedPayloadsDirectly(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	at := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	events := []Event{
		createdEvent(t, ref, at),
		eventWithPayload(t, ref, 2, at, EventIncidentStatus, StatusPayload{Status: "paused"}),
		eventWithPayload(t, ref, 2, at, EventIncidentOutcome, OutcomePayload{Outcome: "duplicate"}),
		eventWithPayload(t, ref, 2, at, EventIncidentMerged, MergedPayload{}),
		eventWithPayload(t, ref, 2, at, EventNoteAdded, NoteAddedPayload{}),
	}
	events[0].Payload = mustJSON(t, CreatedPayload{})
	for _, event := range events {
		require.Error(t, event.Validate(), event.Type)
	}
}

func TestFoldRejectsInvalidStreamsAndPayloads(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	at := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	created := createdEvent(t, ref, at)

	_, err := Fold(nil, nil)
	require.ErrorContains(t, err, "no visible")

	_, err = Fold([]Event{created}, &time.Time{})
	require.ErrorContains(t, err, "no visible")

	gap := created
	gap.Seq = 2
	_, err = Fold([]Event{gap}, nil)
	require.ErrorContains(t, err, "expected sequence")

	notCreated := created
	notCreated.Type = EventNoteAdded
	notCreated.Payload = mustJSON(t, NoteAddedPayload{Body: "hello"})
	_, err = Fold([]Event{notCreated}, nil)
	require.ErrorContains(t, err, "first event")

	wrongIncident := eventWithPayload(t, IncidentRef{StoreID: "ops", IncidentID: "INC-2"}, 2, at.Add(time.Minute), EventNoteAdded, NoteAddedPayload{Body: "hello"})
	_, err = Fold([]Event{created, wrongIncident}, nil)
	require.ErrorContains(t, err, "does not match")

	duplicateCreated := createdEvent(t, ref, at.Add(time.Minute))
	duplicateCreated.Seq = 2
	duplicateCreated.ID = "evt-2"
	_, err = Fold([]Event{created, duplicateCreated}, nil)
	require.ErrorContains(t, err, "only once")

	badJSON := eventWithPayload(t, ref, 2, at.Add(time.Minute), EventIncidentStatus, StatusPayload{Status: StatusResolved})
	badJSON.Payload = json.RawMessage(`{`)
	_, err = Fold([]Event{created, badJSON}, nil)
	require.ErrorContains(t, err, "invalid payload")

	badCreatedJSON := created
	badCreatedJSON.Payload = json.RawMessage(`{`)
	_, err = Fold([]Event{badCreatedJSON}, nil)
	require.ErrorContains(t, err, "invalid payload")

	resolved := eventWithPayload(t, ref, 2, at.Add(time.Minute), EventIncidentStatus, StatusPayload{Status: StatusResolved})
	badOutcomeJSON := eventWithPayload(t, ref, 3, at.Add(2*time.Minute), EventIncidentOutcome, OutcomePayload{Outcome: OutcomeResolved})
	badOutcomeJSON.Payload = json.RawMessage(`{`)
	_, err = Fold([]Event{created, resolved, badOutcomeJSON}, nil)
	require.ErrorContains(t, err, "invalid payload")

	badMergeJSON := eventWithPayload(t, ref, 2, at.Add(time.Minute), EventIncidentMerged, MergedPayload{Into: IncidentRef{StoreID: "ops", IncidentID: "INC-2"}, MergeID: "merge-1"})
	badMergeJSON.Payload = json.RawMessage(`{`)
	_, err = Fold([]Event{created, badMergeJSON}, nil)
	require.ErrorContains(t, err, "invalid payload")

	badMergeRef := eventWithPayload(t, ref, 2, at.Add(time.Minute), EventIncidentMerged, MergedPayload{})
	_, err = Fold([]Event{created, badMergeRef}, nil)
	require.Error(t, err)

	badNoteJSON := eventWithPayload(t, ref, 2, at.Add(time.Minute), EventNoteAdded, NoteAddedPayload{Body: "hello"})
	badNoteJSON.Payload = json.RawMessage(`{`)
	_, err = Fold([]Event{created, badNoteJSON}, nil)
	require.ErrorContains(t, err, "invalid payload")

	unknown := eventWithPayload(t, ref, 2, at.Add(time.Minute), "unknown.event", map[string]string{"x": "y"})
	require.ErrorContains(t, unknown.Validate(), "unsupported event")
	_, err = Fold([]Event{created, unknown}, nil)
	require.ErrorContains(t, err, "unsupported event")

	unknownField := created
	unknownField.Payload = json.RawMessage(`{"uid":"uid","title":"title","admin":true}`)
	require.ErrorContains(t, unknownField.Validate(), "unknown field")

	trailing := created
	trailing.Payload = json.RawMessage(`{"uid":"uid","title":"title"} {}`)
	require.ErrorContains(t, trailing.Validate(), "trailing")

}

func TestFoldLifecycleAndMerge(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	at := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	created := createdEvent(t, ref, at)

	statuses := []Status{StatusInvestigating, StatusMitigating, StatusRecovering, StatusResolved}
	events := []Event{created}
	for i, status := range statuses {
		events = append(events, eventWithPayload(t, ref, uint64(i+2), at.Add(time.Duration(i+1)*time.Minute), EventIncidentStatus, StatusPayload{Status: status}))
	}
	projection, err := Fold(events, nil)
	require.NoError(t, err)
	require.Equal(t, StatusResolved, projection.Status)

	outcome := eventWithPayload(t, ref, uint64(len(events)+1), at.Add(5*time.Minute), EventIncidentOutcome, OutcomePayload{Outcome: OutcomeResolved})
	watching := eventWithPayload(t, ref, uint64(len(events)+2), at.Add(6*time.Minute), EventIncidentStatus, StatusPayload{Status: StatusWatching})
	closed := eventWithPayload(t, ref, uint64(len(events)+3), at.Add(7*time.Minute), EventIncidentStatus, StatusPayload{Status: StatusClosed})
	events = append(events, outcome, watching, closed)
	projection, err = Fold(events, nil)
	require.NoError(t, err)
	require.Equal(t, StatusClosed, projection.Status)

	backward := eventWithPayload(t, ref, uint64(len(events)+1), at.Add(time.Hour), EventIncidentStatus, StatusPayload{Status: StatusOpen})
	_, err = Fold(append(events, backward), nil)
	require.ErrorContains(t, err, "backward")

	badStatus := eventWithPayload(t, ref, 2, at.Add(time.Minute), EventIncidentStatus, StatusPayload{Status: "paused"})
	_, err = Fold([]Event{created, badStatus}, nil)
	require.ErrorContains(t, err, "invalid status")

	merged := eventWithPayload(t, ref, 2, at.Add(time.Minute), EventIncidentMerged, MergedPayload{Into: IncidentRef{StoreID: "ops", IncidentID: "INC-9"}, MergeID: "merge-1"})
	mergedProjection, err := Fold([]Event{created, merged}, nil)
	require.NoError(t, err)
	require.Equal(t, StatusClosed, mergedProjection.Status)
	require.Equal(t, "ops/INC-9", mergedProjection.MergedInto.String())
	require.Empty(t, mergedProjection.Outcome)

	selfMerge := eventWithPayload(t, ref, 2, at.Add(time.Minute), EventIncidentMerged, MergedPayload{Into: ref, MergeID: "merge-1"})
	require.ErrorContains(t, selfMerge.Validate(), "itself")

	noteAfterMerge := eventWithPayload(t, ref, 3, at.Add(2*time.Minute), EventNoteAdded, NoteAddedPayload{Body: "too late"})
	_, err = Fold([]Event{created, merged, noteAfterMerge}, nil)
	require.ErrorContains(t, err, "terminal")

	closeWithoutOutcome := eventWithPayload(t, ref, 2, at.Add(time.Minute), EventIncidentStatus, StatusPayload{Status: StatusClosed})
	_, err = Fold([]Event{created, closeWithoutOutcome}, nil)
	require.ErrorContains(t, err, "requires an outcome")
}

func TestFoldCreatedAndOutcomeValidation(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	at := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	created := createdEvent(t, ref, at)

	badCreated := created
	badCreated.Payload = mustJSON(t, CreatedPayload{})
	_, err := Fold([]Event{badCreated}, nil)
	require.ErrorContains(t, err, "uid and title")

	badProject := created
	badProject.Payload = mustJSON(t, CreatedPayload{UID: "uid", Title: "title", Projects: []ProjectRef{{StoreID: "ops"}}})
	_, err = Fold([]Event{badProject}, nil)
	require.ErrorContains(t, err, "project 0")

	resolved := eventWithPayload(t, ref, 2, at.Add(time.Minute), EventIncidentStatus, StatusPayload{Status: StatusResolved})
	for _, outcome := range []Outcome{OutcomeResolved, OutcomeFalseAlarm, OutcomeAccepted, OutcomeHandedOff, OutcomeUnresolved} {
		outcomeEvent := eventWithPayload(t, ref, 3, at.Add(2*time.Minute), EventIncidentOutcome, OutcomePayload{Outcome: outcome})
		projection, foldErr := Fold([]Event{created, resolved, outcomeEvent}, nil)
		require.NoError(t, foldErr)
		require.Equal(t, outcome, projection.Outcome)
	}

	badOutcome := eventWithPayload(t, ref, 3, at.Add(2*time.Minute), EventIncidentOutcome, OutcomePayload{Outcome: "duplicate"})
	_, err = Fold([]Event{created, resolved, badOutcome}, nil)
	require.ErrorContains(t, err, "invalid outcome")

	firstOutcome := eventWithPayload(t, ref, 3, at.Add(2*time.Minute), EventIncidentOutcome, OutcomePayload{Outcome: OutcomeResolved})
	secondOutcome := eventWithPayload(t, ref, 4, at.Add(3*time.Minute), EventIncidentOutcome, OutcomePayload{Outcome: OutcomeAccepted})
	_, err = Fold([]Event{created, resolved, firstOutcome, secondOutcome}, nil)
	require.ErrorContains(t, err, "already recorded")

	emptyNote := eventWithPayload(t, ref, 2, at.Add(time.Minute), EventNoteAdded, NoteAddedPayload{})
	_, err = Fold([]Event{created, emptyNote}, nil)
	require.ErrorContains(t, err, "note body")
}
