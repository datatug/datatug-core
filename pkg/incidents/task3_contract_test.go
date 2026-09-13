package incidents

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/investigation"
	"github.com/stretchr/testify/require"
)

func TestCreateMutationProjectsReporterAndCanonicalFacts(t *testing.T) {
	mutation := task3CreateMutation()
	require.NoError(t, mutation.Validate())

	event := task3CreatedEvent(mutation)
	projection, err := Fold([]Event{event}, nil)
	require.NoError(t, err)
	require.Equal(t, []Participant{{Actor: mutation.Reporter, Role: ParticipantReporter}}, projection.Participants)
	require.Equal(t, mutation.CanonicalContext, projection.CanonicalContext)

	bad := mutation
	bad.Reporter = Actor{}
	require.Error(t, bad.Validate())
	bad = mutation
	bad.CanonicalContext.Facts[0].Value = investigation.NewIntegerValue("007")
	require.Error(t, bad.Validate())
}

func TestApplyViewPolicyRedactsWithoutMutatingStoredEvents(t *testing.T) {
	mutation := task3CreateMutation()
	stored := task3CreatedEvent(mutation)
	view, visible, err := ApplyEventView(stored, ViewPolicy{Facts: map[string]FactVisibility{
		"customer-email": FactValueRedacted,
	}})
	require.NoError(t, err)
	require.True(t, visible)

	var payload CreatedViewPayload
	require.NoError(t, json.Unmarshal(view.Payload, &payload))
	require.True(t, payload.CanonicalContext.Facts[0].Value.Redacted)

	var original CreatedPayload
	require.NoError(t, json.Unmarshal(stored.Payload, &original))
	require.Equal(t, "a@b.com", original.CanonicalContext.Facts[0].Value.Str)

	_, visible, err = ApplyEventView(stored, ViewPolicy{WithheldEvents: map[string]bool{stored.ID: true}})
	require.NoError(t, err)
	require.False(t, visible)

	projection, err := Fold([]Event{stored}, nil)
	require.NoError(t, err)
	redacted := ApplyIncidentView(projection, ViewPolicy{Facts: map[string]FactVisibility{"customer-email": FactValueRedacted}})
	require.True(t, redacted.CanonicalContext.Facts[0].Value.Redacted)
	require.Equal(t, "a@b.com", projection.CanonicalContext.Facts[0].Value.Str)
	hidden := ApplyIncidentView(projection, ViewPolicy{Facts: map[string]FactVisibility{"customer-email": FactHidden}})
	require.Empty(t, hidden.CanonicalContext.Facts)
}

func TestWithheldNoteIsAbsentFromEventWatchAndProjection(t *testing.T) {
	created := task3CreatedEvent(task3CreateMutation())
	note := Event{
		ID: "note-secret", Seq: 2, At: created.At.Add(time.Minute), VisibleAt: created.At.Add(time.Minute),
		Incident: created.Incident, Actor: created.Actor, Type: EventNoteAdded,
		Assertion: Assertion{Kind: AssertionClaim}, Payload: mustTask3JSON(NoteAddedPayload{Body: "customer secret"}),
	}
	projection, err := Fold([]Event{created, note}, nil)
	require.NoError(t, err)
	require.Equal(t, []Note{{EventID: note.ID, Body: "customer secret"}}, projection.NoteEntries)
	require.Equal(t, []string{"customer secret"}, ApplyIncidentView(projection, ViewPolicy{}).Notes)

	policy := ViewPolicy{WithheldEvents: map[string]bool{note.ID: true}}
	_, visible, err := ApplyEventView(note, policy)
	require.NoError(t, err)
	require.False(t, visible)
	_, visible, err = ApplyStreamItemView(StreamItem{Cursor: "cursor-2", Event: note}, policy)
	require.NoError(t, err)
	require.False(t, visible)
	require.Empty(t, ApplyIncidentView(projection, policy).Notes)

	legacy := projection
	legacy.NoteEntries = nil
	require.Empty(t, ApplyIncidentView(legacy, policy).Notes)
	require.Equal(t, []string{"customer secret"}, ApplyIncidentView(legacy, ViewPolicy{}).Notes)
}

func TestLegacyCreatedEventWithoutReporterOrContextStillFolds(t *testing.T) {
	mutation := task3CreateMutation()
	event := task3CreatedEvent(mutation)
	event.Payload = mustTask3JSON(struct {
		UID         string       `json:"uid"`
		Title       string       `json:"title"`
		Description string       `json:"description"`
		Projects    []ProjectRef `json:"projects,omitempty"`
	}{UID: mutation.UID, Title: mutation.Title, Description: mutation.Description, Projects: mutation.Projects})
	require.NoError(t, event.Validate())
	projection, err := Fold([]Event{event}, nil)
	require.NoError(t, err)
	require.Empty(t, projection.Participants)
	require.Empty(t, projection.CanonicalContext.Facts)
}

func TestIncidentNoteEntriesMustRemainCanonical(t *testing.T) {
	projection := task3Projection("INC-1", "Invoices stuck", StatusOpen)
	projection.Notes = []string{"first", "second"}
	projection.NoteEntries = []Note{{EventID: "note-1", Body: "first"}, {EventID: "note-2", Body: "second"}}
	require.NoError(t, projection.Validate())

	for _, mutate := range []func(*Incident){
		func(i *Incident) { i.NoteEntries[0].EventID = "../bad" },
		func(i *Incident) { i.NoteEntries[0].Body = "" },
		func(i *Incident) { i.NoteEntries[1].EventID = "note-1" },
		func(i *Incident) { i.Notes = i.Notes[:1] },
		func(i *Incident) { i.Notes[0] = "changed" },
	} {
		candidate := projection
		candidate.Notes = append([]string(nil), projection.Notes...)
		candidate.NoteEntries = append([]Note(nil), projection.NoteEntries...)
		mutate(&candidate)
		require.Error(t, candidate.Validate())
	}
}

func TestListSearchAndSimilarityAreDeterministicAndExplainable(t *testing.T) {
	base := task3Projection("INC-1", "Canadian invoices stuck", StatusResolved)
	base.AssetRefs = []ArtifactRef{{Kind: RefCheck, Artifact: &ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", ID: "stuck-invoices"}}}
	recurrence := task3Projection("INC-2", "Invoice totals missing again", StatusOpen)
	recurrence.AssetRefs = append([]ArtifactRef(nil), base.AssetRefs...)
	unrelated := task3Projection("INC-3", "Canadian carrier delay", StatusOpen)
	unrelated.CanonicalContext.Facts[0].Value = investigation.NewIntegerValue("99")

	query := ListQuery{Statuses: []Status{StatusOpen}, CheckID: "stuck-invoices"}
	require.NoError(t, query.Validate())
	require.True(t, MatchesListQuery(recurrence, query))
	require.False(t, MatchesListQuery(base, query))
	require.False(t, MatchesListQuery(unrelated, query))

	unrelatedView := ApplyIncidentView(unrelated, ViewPolicy{})
	recurrenceView := ApplyIncidentView(recurrence, ViewPolicy{})
	baseView := ApplyIncidentView(base, ViewPolicy{})
	hits := Search([]IncidentView{unrelatedView, recurrenceView, baseView}, SearchQuery{Text: "invoice", Facts: []FactSignal{{Entity: "Customer", Field: "Email", Value: investigation.NewStringValue("a@b.com")}}})
	require.Equal(t, []IncidentRef{base.Ref, recurrence.Ref}, []IncidentRef{hits[0].Incident.Ref, hits[1].Incident.Ref})
	require.NotEmpty(t, hits[0].MatchedSignals)
	factOnly := Search([]IncidentView{baseView}, SearchQuery{Facts: []FactSignal{{Entity: "Customer", Field: "Email", Value: investigation.NewStringValue("a@b.com")}}})
	require.Len(t, factOnly, 1)
	redactedView := ApplyIncidentView(base, ViewPolicy{Facts: map[string]FactVisibility{"customer-email": FactValueRedacted}})
	require.Empty(t, Search([]IncidentView{redactedView}, SearchQuery{Facts: []FactSignal{{Entity: "Customer", Field: "Email", Value: investigation.NewStringValue("a@b.com")}}}))

	matches := Similar(recurrenceView, []IncidentView{unrelatedView, baseView, recurrenceView})
	require.Equal(t, base.Ref, matches[0].Incident.Ref)
	require.Greater(t, matches[0].Score, matches[1].Score)
	require.NotEmpty(t, matches[0].MatchedSignals)
}

func TestCursorAndWatchContracts(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	cursor := EventCursor("opaque.cursor-2")
	require.NoError(t, cursor.Validate())
	require.Error(t, EventCursor("bad cursor").Validate())

	query := WatchQuery{Incident: &ref, Since: cursor}
	require.NoError(t, query.Validate())
	bad := query
	bad.Incident = &IncidentRef{}
	require.Error(t, bad.Validate())

	item := StreamItem{Cursor: cursor, Event: task3CreatedEvent(task3CreateMutation())}
	require.NoError(t, item.Validate())
	item.Cursor = ""
	require.Error(t, item.Validate())

	var _ EventStream = (*stubEventStream)(nil)
	var _ Store = (*task3StubStore)(nil)
}

type stubEventStream struct{}

func (*stubEventStream) Next(context.Context) (StreamItem, error) { return StreamItem{}, io.EOF }
func (*stubEventStream) Close() error                             { return nil }

type task3StubStore struct{}

func (*task3StubStore) Create(context.Context, CreateMutation) (CreateResult, error) {
	return CreateResult{}, nil
}
func (*task3StubStore) Append(context.Context, Mutation) (AppendResult, error) {
	return AppendResult{}, nil
}
func (*task3StubStore) Events(context.Context, IncidentRef, uint64) ([]Event, error) { return nil, nil }
func (*task3StubStore) Projection(context.Context, IncidentRef, *time.Time) (Incident, error) {
	return Incident{}, nil
}
func (*task3StubStore) List(context.Context, ListQuery) ([]Incident, error) { return nil, nil }
func (*task3StubStore) Watch(context.Context, WatchQuery) (EventStream, error) {
	return &stubEventStream{}, nil
}
func (*task3StubStore) Merge(context.Context, MergeMutation) (MergeResult, error) {
	return MergeResult{}, nil
}

func task3CreateMutation() CreateMutation {
	return CreateMutation{
		MutationID: "create-1", StoreID: "ops", UID: "uid-1", Title: "Canadian invoices stuck",
		Description: "customer 5 affected", At: time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC),
		Reporter: Actor{Kind: ActorHuman, ID: "alex", Via: ActorViaAPI},
		Projects: []ProjectRef{{StoreID: "ops", ProjectID: "billing", Environment: "prod"}},
		CanonicalContext: investigation.Context{Facts: []investigation.Fact{{
			ID: "customer-email", Entity: "Customer", Field: "Email",
			Value: investigation.NewStringValue("a@b.com"), Origin: investigation.FactOriginManual,
			Enabled: true, Layer: "canonical",
			Physical: &investigation.PhysicalRef{Source: "chinook", Collection: "Customer", Column: "Email"},
		}}},
	}
}

func task3CreatedEvent(mutation CreateMutation) Event {
	return Event{
		ID: mutation.MutationID, Seq: 1, At: mutation.At, VisibleAt: mutation.At,
		Incident: IncidentRef{StoreID: mutation.StoreID, IncidentID: "INC-1"},
		Actor:    mutation.Reporter, Type: EventIncidentCreated, Assertion: Assertion{Kind: AssertionClaim},
		Payload: mustTask3JSON(CreatedPayload{
			UID: mutation.UID, Title: mutation.Title, Description: mutation.Description, Projects: mutation.Projects,
			Reporter: mutation.Reporter, CanonicalContext: mutation.CanonicalContext,
		}),
	}
}

func task3Projection(id, title string, status Status) Incident {
	mutation := task3CreateMutation()
	mutation.Title = title
	projection, err := Fold([]Event{task3CreatedEvent(mutation)}, nil)
	if err != nil {
		panic(err)
	}
	projection.Ref.IncidentID = id
	projection.Status = status
	return projection
}

func mustTask3JSON(value any) json.RawMessage {
	b, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return b
}
