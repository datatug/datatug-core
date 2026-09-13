package incidents

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/investigation"
	"github.com/stretchr/testify/require"
)

func TestTask3ProjectionAndViewValidationFailures(t *testing.T) {
	valid := task3Projection("INC-1", "Invoices stuck", StatusOpen)
	require.NoError(t, valid.Validate())
	valid.Participants = []Participant{{Actor: Actor{Kind: ActorHuman, ID: "alex"}, Role: ParticipantReporter}}
	require.NoError(t, valid.Validate())

	mutations := []func(*Incident){
		func(i *Incident) { i.Ref = IncidentRef{} },
		func(i *Incident) { i.UID = "" },
		func(i *Incident) { i.Outcome = "duplicate" },
		func(i *Incident) { i.MergedInto = &IncidentRef{} },
		func(i *Incident) { merged := i.Ref; i.MergedInto = &merged },
		func(i *Incident) { i.Projects = []ProjectRef{{StoreID: "ops"}} },
		func(i *Incident) { i.AssetRefs = []ArtifactRef{{Kind: RefQuery}} },
		func(i *Incident) { i.Participants[0].Role = "owner" },
		func(i *Incident) { i.Participants[0].Actor.Kind = "robot" },
		func(i *Incident) { i.CanonicalContext.Facts[0].ID = "" },
	}
	for _, mutate := range mutations {
		candidate := valid
		candidate.Participants = append([]Participant(nil), valid.Participants...)
		candidate.CanonicalContext.Facts = append([]investigation.Fact(nil), valid.CanonicalContext.Facts...)
		mutate(&candidate)
		require.Error(t, candidate.Validate())
	}

	view := ApplyIncidentView(valid, ViewPolicy{})
	require.NoError(t, view.Validate())
	badView := view
	badView.Title = ""
	require.Error(t, badView.Validate())
	badView = view
	badView.CanonicalContext.Facts[0].ID = ""
	require.Error(t, badView.Validate())
}

func TestTask3CreateMutationValidationFailures(t *testing.T) {
	valid := task3CreateMutation()
	mutations := []func(*CreateMutation){
		func(m *CreateMutation) { m.MutationID = "../bad" },
		func(m *CreateMutation) { m.Title = "" },
		func(m *CreateMutation) { m.Reporter.ID = "" },
		func(m *CreateMutation) { m.Projects[0].ProjectID = "" },
		func(m *CreateMutation) { m.CanonicalContext.Facts[0].Value = investigation.NewIntegerValue("01") },
	}
	for _, mutate := range mutations {
		candidate := valid
		candidate.Projects = append([]ProjectRef(nil), valid.Projects...)
		candidate.CanonicalContext.Facts = append([]investigation.Fact(nil), valid.CanonicalContext.Facts...)
		mutate(&candidate)
		require.Error(t, candidate.Validate())
	}
}

func TestTask3CreatedViewValidationFailures(t *testing.T) {
	event := task3CreatedEvent(task3CreateMutation())
	view, visible, err := ApplyEventView(event, ViewPolicy{})
	require.NoError(t, err)
	require.True(t, visible)
	require.NoError(t, view.ValidateView())

	badStored := event
	badStored.Payload = json.RawMessage(`{"uid":`)
	_, _, err = ApplyEventView(badStored, ViewPolicy{})
	require.Error(t, err)

	var payload CreatedViewPayload
	require.NoError(t, json.Unmarshal(view.Payload, &payload))
	mutations := []func(*CreatedViewPayload){
		func(p *CreatedViewPayload) { p.Title = "" },
		func(p *CreatedViewPayload) { p.Projects = []ProjectRef{{StoreID: "ops"}} },
		func(p *CreatedViewPayload) { p.Reporter.Kind = "robot" },
		func(p *CreatedViewPayload) { p.CanonicalContext.Facts[0].ID = "" },
	}
	for _, mutate := range mutations {
		candidate := payload
		candidate.CanonicalContext.Facts = append([]investigation.FactView(nil), payload.CanonicalContext.Facts...)
		mutate(&candidate)
		bad := view
		bad.Payload = mustTask3JSON(candidate)
		require.Error(t, bad.ValidateView())
	}
	bad := view
	bad.Payload = json.RawMessage(`{"uid":`)
	require.Error(t, bad.ValidateView())

	var storedPayload CreatedPayload
	require.NoError(t, json.Unmarshal(event.Payload, &storedPayload))
	storedPayload.Reporter.Kind = "robot"
	bad = event
	bad.Payload = mustTask3JSON(storedPayload)
	require.Error(t, bad.Validate())
	storedPayload.Reporter = event.Actor
	storedPayload.CanonicalContext.Facts[0].ID = ""
	bad.Payload = mustTask3JSON(storedPayload)
	require.Error(t, bad.Validate())
}

func TestTask3ListAndSearchValidation(t *testing.T) {
	incident := task3Projection("INC-1", "Invoice alert", StatusOpen)
	incident.Projects = []ProjectRef{{StoreID: "ops", ProjectID: "billing"}}
	incident.AssetRefs = []ArtifactRef{
		{Kind: RefQuery, Artifact: &ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", ID: "invoice-query"}},
		{Kind: RefCheck, Artifact: &ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", ID: "invoice-check"}},
		{Kind: RefBoard, Artifact: &ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", ID: "invoice-board"}},
	}
	require.True(t, MatchesListQuery(incident, ListQuery{Statuses: []Status{StatusOpen}, ProjectID: "billing", QueryID: "invoice-query", CheckID: "invoice-check", BoardID: "invoice-board"}))
	require.False(t, MatchesListQuery(incident, ListQuery{ProjectID: "other"}))
	require.False(t, MatchesListQuery(incident, ListQuery{QueryID: "other"}))
	require.False(t, MatchesListQuery(incident, ListQuery{BoardID: "other"}))

	for _, query := range []ListQuery{
		{Statuses: []Status{"paused"}}, {ProjectID: " bad"}, {QueryID: "bad\nvalue"}, {CheckID: ""},
	} {
		if query.CheckID == "" && len(query.Statuses) == 0 && query.ProjectID == "" && query.QueryID == "" {
			require.NoError(t, query.Validate())
		} else {
			require.Error(t, query.Validate())
		}
	}

	require.Error(t, (FactSignal{}).Validate())
	require.Error(t, (FactSignal{Entity: "Customer", Field: "ID", Value: investigation.NewIntegerValue("01")}).Validate())
	require.Error(t, (SearchQuery{}).Validate())
	require.Error(t, (SearchQuery{Facts: []FactSignal{{}}}).Validate())
	require.Empty(t, Search(nil, SearchQuery{}))

	view := ApplyIncidentView(incident, ViewPolicy{})
	require.Empty(t, Search([]IncidentView{view}, SearchQuery{Text: "unmatched"}))
	require.Empty(t, Search([]IncidentView{view}, SearchQuery{Text: "invoice", Facts: []FactSignal{{Entity: "Customer", Field: "Email", Value: investigation.NewStringValue("other")}}}))
}

func TestTask3MatchValidationAndSimilaritySignals(t *testing.T) {
	view := ApplyIncidentView(task3Projection("INC-1", "Canadian invoice failure", StatusOpen), ViewPolicy{})
	for _, kind := range []SignalKind{SignalText, SignalFact, SignalQuery, SignalCheck, SignalBoard, SignalProject} {
		require.NoError(t, (MatchedSignal{Kind: kind, Value: "match"}).Validate())
	}
	require.Error(t, (MatchedSignal{Kind: "metric", Value: "match"}).Validate())
	require.Error(t, (MatchedSignal{Kind: SignalText}).Validate())

	validSignal := []MatchedSignal{{Kind: SignalText, Value: "invoice"}}
	require.NoError(t, (SearchMatch{Incident: view, MatchedSignals: validSignal}).Validate())
	require.Error(t, (SearchMatch{Incident: IncidentView{}, MatchedSignals: validSignal}).Validate())
	require.Error(t, (SearchMatch{Incident: view}).Validate())
	require.Error(t, (SearchMatch{Incident: view, MatchedSignals: []MatchedSignal{{Kind: "bad", Value: "x"}}}).Validate())
	require.NoError(t, (SimilarMatch{Incident: view, Score: 1, MatchedSignals: validSignal}).Validate())
	require.Error(t, (SimilarMatch{Incident: IncidentView{}, Score: 1, MatchedSignals: validSignal}).Validate())
	require.Error(t, (SimilarMatch{Incident: view, MatchedSignals: validSignal}).Validate())

	subject := view
	subject.AssetRefs = []ArtifactRef{
		{Kind: RefQuery, Artifact: &ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", ID: "q"}},
		{Kind: RefBoard, Artifact: &ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", ID: "b"}},
		{Kind: RefExecution, Artifact: &ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", ID: "ignored"}},
		{Kind: RefCheck},
	}
	subject.Projects = []ProjectRef{{StoreID: "ops", ProjectID: "billing"}}
	candidateA := subject
	candidateA.Ref.IncidentID = "INC-3"
	candidateB := subject
	candidateB.Ref.IncidentID = "INC-2"
	matches := Similar(subject, []IncidentView{subject, candidateA, candidateB})
	require.Len(t, matches, 2)
	require.Equal(t, "INC-2", matches[0].Incident.Ref.IncidentID)
	require.Contains(t, matches[0].MatchedSignals, MatchedSignal{Kind: SignalQuery, Value: "q"})
	require.Contains(t, matches[0].MatchedSignals, MatchedSignal{Kind: SignalBoard, Value: "b"})

	unrelated := candidateA
	unrelated.Title, unrelated.Description = "other", "none"
	unrelated.CanonicalContext = investigation.ContextView{}
	unrelated.AssetRefs, unrelated.Projects = nil, nil
	require.Empty(t, Similar(subject, []IncidentView{unrelated}))
	redacted := subject
	redacted.CanonicalContext = investigation.ContextView{Facts: []investigation.FactView{investigation.RedactedFact(task3CreateMutation().CanonicalContext.Facts[0], true)}}
	_, _ = similaritySignals(redacted, unrelated)
	require.False(t, hasExactProject(nil, ProjectRef{StoreID: "ops", ProjectID: "billing"}))
	require.False(t, hasMatchingAsset(nil, ArtifactRef{}))
}

func TestTask3CursorAndStreamPolicyFailures(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	require.NoError(t, (WatchQuery{}).Validate())
	require.Error(t, (WatchQuery{Incident: &ref, Since: EventCursor(strings.Repeat("x", 2049))}).Validate())
	require.Error(t, EventCursor("bad\tvalue").Validate())

	event := task3CreatedEvent(task3CreateMutation())
	stored := StreamItem{Cursor: "cursor-1", Event: event}
	view, visible, err := ApplyStreamItemView(stored, ViewPolicy{})
	require.NoError(t, err)
	require.True(t, visible)
	require.Equal(t, stored.Cursor, view.Cursor)

	bad := stored
	bad.Event.Payload = json.RawMessage(`{"uid":`)
	_, visible, err = ApplyStreamItemView(bad, ViewPolicy{})
	require.Error(t, err)
	require.False(t, visible)
}

func TestTask3FoldCollectsUniqueAssetBacklinks(t *testing.T) {
	created := task3CreatedEvent(task3CreateMutation())
	ref := ArtifactRef{Kind: RefQuery, Artifact: &ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", ID: "invoice-query"}}
	created.Refs = []ArtifactRef{ref}
	note := Event{
		ID: "note-1", Seq: 2, At: created.At, VisibleAt: created.At, Incident: created.Incident,
		Actor: created.Actor, Type: EventNoteAdded, Assertion: Assertion{Kind: AssertionClaim}, Refs: []ArtifactRef{ref},
		Payload: mustTask3JSON(NoteAddedPayload{Body: "checked"}),
	}
	projection, err := Fold([]Event{created, note}, nil)
	require.NoError(t, err)
	require.Equal(t, []ArtifactRef{ref}, projection.AssetRefs)
}
