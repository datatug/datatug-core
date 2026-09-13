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
	factKey := mutation.CanonicalContext.Facts[0].Key()
	view, visible, err := ApplyEventView(stored, ViewPolicy{Facts: map[investigation.FactKey]FactVisibility{
		factKey: FactValueRedacted,
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
	redacted := ApplyIncidentView(projection, ViewPolicy{Facts: map[investigation.FactKey]FactVisibility{factKey: FactValueRedacted}})
	require.True(t, redacted.CanonicalContext.Facts[0].Value.Redacted)
	require.Equal(t, "a@b.com", projection.CanonicalContext.Facts[0].Value.Str)
	hidden := ApplyIncidentView(projection, ViewPolicy{Facts: map[investigation.FactKey]FactVisibility{factKey: FactHidden}})
	require.Empty(t, hidden.CanonicalContext.Facts)
}

func TestFactPolicyIsScopeQualifiedAndFailClosed(t *testing.T) {
	stored := task3Projection("INC-1", "Scoped customer issue", StatusOpen)
	first := stored.CanonicalContext.Facts[0]
	second := first
	secondScope := investigation.ProjectScope{StoreID: "warehouse", ProjectID: "billing", Environment: "prod"}
	second.Scope = &secondScope
	stored.CanonicalContext.Facts = []investigation.Fact{first, second}
	require.NoError(t, stored.CanonicalContext.Validate())

	policy := ViewPolicy{Facts: map[investigation.FactKey]FactVisibility{first.Key(): FactVisible}}
	view := ApplyIncidentView(stored, policy)
	require.Equal(t, []investigation.FactView{investigation.VisibleFact(first)}, view.CanonicalContext.Facts)

	created := task3CreatedEvent(task3CreateMutation())
	var createdPayload CreatedPayload
	require.NoError(t, json.Unmarshal(created.Payload, &createdPayload))
	createdPayload.CanonicalContext.Facts = []investigation.Fact{first, second}
	created.Payload = mustTask3JSON(createdPayload)
	eventView, visible, err := ApplyEventView(created, policy)
	require.NoError(t, err)
	require.True(t, visible)
	var visiblePayload CreatedViewPayload
	require.NoError(t, json.Unmarshal(eventView.Payload, &visiblePayload))
	require.Equal(t, view.CanonicalContext, visiblePayload.CanonicalContext)

	query := SearchQuery{Facts: []FactSignal{{Entity: second.Entity, Field: second.Field, Value: second.Value}}}
	require.Len(t, Search([]IncidentView{view}, query), 1) // the permitted identical fact remains searchable
	hiddenOnly := ApplyIncidentView(Incident{CanonicalContext: investigation.Context{Facts: []investigation.Fact{second}}}, policy)
	require.Empty(t, Search([]IncidentView{hiddenOnly}, query))
	require.Empty(t, Similar(hiddenOnly, []IncidentView{view}))
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

func TestWithheldAssetBacklinksUseStableEventProvenance(t *testing.T) {
	created := task3CreatedEvent(task3CreateMutation())
	checkRef := ArtifactRef{Kind: RefCheck, Artifact: &ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", Environment: "prod", ID: "invoice-health"}}
	first := Event{
		ID: "note-asset-1", Seq: 2, At: created.At.Add(time.Minute), VisibleAt: created.At.Add(time.Minute),
		Incident: created.Incident, Actor: created.Actor, Type: EventNoteAdded, Assertion: Assertion{Kind: AssertionClaim},
		Refs: []ArtifactRef{checkRef, checkRef}, Payload: mustTask3JSON(NoteAddedPayload{Body: "first check"}),
	}
	second := first
	second.ID, second.Seq, second.At, second.VisibleAt = "note-asset-2", 3, first.At.Add(time.Minute), first.VisibleAt.Add(time.Minute)
	second.Payload = mustTask3JSON(NoteAddedPayload{Body: "check repeated"})

	projection, err := Fold([]Event{created, first, second}, nil)
	require.NoError(t, err)
	require.Equal(t, []ArtifactRef{checkRef}, projection.AssetRefs)
	require.Equal(t, []AssetRefEntry{{EventID: first.ID, Ref: checkRef}, {EventID: second.ID, Ref: checkRef}}, projection.AssetRefEntries)

	firstWithheld := ViewPolicy{WithheldEvents: map[string]bool{first.ID: true}}
	oneVisible := ApplyIncidentView(projection, firstWithheld)
	require.Equal(t, []ArtifactRef{checkRef}, oneVisible.AssetRefs)
	bothWithheld := ViewPolicy{WithheldEvents: map[string]bool{first.ID: true, second.ID: true}}
	noneVisible := ApplyIncidentView(projection, bothWithheld)
	require.Empty(t, noneVisible.AssetRefs)

	candidate := IncidentView{
		Ref:       IncidentRef{StoreID: "ops", IncidentID: "INC-2"},
		AssetRefs: []ArtifactRef{checkRef},
	}
	oneVisibleMatches := Similar(oneVisible, []IncidentView{candidate})
	require.Len(t, oneVisibleMatches, 1)
	require.Equal(t, 3, oneVisibleMatches[0].Score)
	require.Equal(t, []MatchedSignal{{Kind: SignalCheck, Value: "invoice-health"}}, oneVisibleMatches[0].MatchedSignals)
	require.Empty(t, Similar(noneVisible, []IncidentView{candidate}))
	listQuery := ListQuery{ProjectStoreID: "ops", ProjectID: "billing", Environment: "prod", CheckID: "invoice-health"}
	require.True(t, MatchesListQuery(oneVisible, listQuery))
	require.False(t, MatchesListQuery(noneVisible, listQuery))

	legacy := projection
	legacy.AssetRefEntries = nil
	require.Empty(t, ApplyIncidentView(legacy, firstWithheld).AssetRefs)
	require.Equal(t, []ArtifactRef{checkRef}, ApplyIncidentView(legacy, ViewPolicy{}).AssetRefs)
}

func TestAssetRefEntriesMustRemainCanonical(t *testing.T) {
	projection := task3Projection("INC-1", "Invoices stuck", StatusOpen)
	first := ArtifactRef{Kind: RefCheck, Artifact: &ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", ID: "check-1"}}
	second := ArtifactRef{Kind: RefQuery, Artifact: &ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", ID: "query-1"}}
	projection.AssetRefs = []ArtifactRef{first, second}
	projection.AssetRefEntries = []AssetRefEntry{{EventID: "event-1", Ref: first}, {EventID: "event-2", Ref: first}, {EventID: "event-2", Ref: second}}
	require.NoError(t, projection.Validate())

	for _, mutate := range []func(*Incident){
		func(i *Incident) { i.AssetRefEntries[0].EventID = "../bad" },
		func(i *Incident) { i.AssetRefEntries[1].EventID = "event-1" },
		func(i *Incident) { i.AssetRefEntries[2].Ref = ArtifactRef{} },
		func(i *Incident) { i.AssetRefEntries[2].Ref = ArtifactRef{Kind: RefQuery} },
		func(i *Incident) { i.AssetRefs = i.AssetRefs[:1] },
		func(i *Incident) { i.AssetRefs[0], i.AssetRefs[1] = i.AssetRefs[1], i.AssetRefs[0] },
	} {
		candidate := projection
		candidate.AssetRefs = cloneArtifactRefs(projection.AssetRefs)
		candidate.AssetRefEntries = append([]AssetRefEntry(nil), projection.AssetRefEntries...)
		mutate(&candidate)
		require.Error(t, candidate.Validate())
	}
}

func TestReadViewsAreDeeplyDetached(t *testing.T) {
	stored := task3Projection("INC-1", "Detached views", StatusClosed)
	mergedInto := IncidentRef{StoreID: "ops", IncidentID: "INC-2"}
	stored.MergedInto = &mergedInto
	stored.AssetRefs = allArtifactRefShapes()
	stored.CanonicalContext.Facts[0].Physical = &investigation.PhysicalRef{Source: "crm", Collection: "Customer", Column: "Email"}
	policy := ViewPolicy{Facts: map[investigation.FactKey]FactVisibility{stored.CanonicalContext.Facts[0].Key(): FactVisible}}
	view := ApplyIncidentView(stored, policy)

	view.MergedInto.IncidentID = "mutated"
	view.CanonicalContext.Facts[0].Physical.Source = "mutated"
	view.CanonicalContext.Facts[0].Scope.StoreID = "mutated"
	mutateAllArtifactRefs(view.AssetRefs)
	require.Equal(t, "INC-2", stored.MergedInto.IncidentID)
	require.Equal(t, "crm", stored.CanonicalContext.Facts[0].Physical.Source)
	require.Equal(t, "ops", stored.CanonicalContext.Facts[0].Scope.StoreID)
	requireAllArtifactRefsUnchanged(t, stored.AssetRefs)

	payload := mustTask3JSON(NoteAddedPayload{Body: "detached"})
	event := Event{ID: "note-detached", Seq: 2, At: time.Now().UTC(), VisibleAt: time.Now().UTC(), Incident: stored.Ref,
		Actor: task3CreateMutation().Reporter, Type: EventNoteAdded, Assertion: Assertion{Kind: AssertionClaim},
		ImportedFrom: &ImportedEventRef{Incident: IncidentRef{StoreID: "ops", IncidentID: "INC-9"}, EventID: "source-1", Seq: 1, MergeID: "merge-1"},
		Refs:         allArtifactRefShapes(), Payload: payload}
	eventView, visible, err := ApplyEventView(event, ViewPolicy{})
	require.NoError(t, err)
	require.True(t, visible)
	eventView.Payload[0] = 'X'
	eventView.ImportedFrom.EventID = "mutated"
	mutateAllArtifactRefs(eventView.Refs)
	require.JSONEq(t, `{"body":"detached"}`, string(event.Payload))
	require.Equal(t, "source-1", event.ImportedFrom.EventID)
	requireAllArtifactRefsUnchanged(t, event.Refs)
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

func TestCreateMutationBindsFactScopesToDeclaredProjects(t *testing.T) {
	mutation := task3CreateMutation()
	secondary := ProjectRef{StoreID: "warehouse", ProjectID: "payments", Environment: "staging"}
	mutation.Projects = append(mutation.Projects, secondary)
	secondFact := mutation.CanonicalContext.Facts[0]
	secondFact.Scope = &secondary
	mutation.CanonicalContext.Facts = append(mutation.CanonicalContext.Facts, secondFact)
	require.NoError(t, mutation.Validate())

	for _, mutate := range []func(*CreateMutation){
		func(m *CreateMutation) { m.CanonicalContext.Facts[0].Scope.StoreID = "foreign" },
		func(m *CreateMutation) { m.CanonicalContext.Facts[0].Scope.ProjectID = "undeclared" },
		func(m *CreateMutation) { m.CanonicalContext.Facts[0].Scope.Environment = "staging" },
	} {
		candidate := mutation
		candidate.CanonicalContext.Facts = cloneFacts(mutation.CanonicalContext.Facts)
		mutate(&candidate)
		require.Error(t, candidate.Validate())
	}
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
	listScope := ProjectRef{StoreID: "ops", ProjectID: "billing", Environment: "prod"}
	base := task3Projection("INC-1", "Canadian invoices stuck", StatusResolved)
	base.Projects = []ProjectRef{listScope}
	base.AssetRefs = []ArtifactRef{{Kind: RefCheck, Artifact: &ProjectArtifactRef{StoreID: listScope.StoreID, ProjectID: listScope.ProjectID, Environment: listScope.Environment, ID: "stuck-invoices"}}}
	recurrence := task3Projection("INC-2", "Invoice totals missing again", StatusOpen)
	recurrence.Projects = []ProjectRef{listScope}
	recurrence.AssetRefs = append([]ArtifactRef(nil), base.AssetRefs...)
	unrelated := task3Projection("INC-3", "Canadian carrier delay", StatusOpen)
	unrelated.CanonicalContext.Facts[0].Value = investigation.NewIntegerValue("99")
	unrelatedView := ApplyIncidentView(unrelated, visiblePolicyFor(unrelated.CanonicalContext.Facts...))
	recurrenceView := ApplyIncidentView(recurrence, visiblePolicyFor(recurrence.CanonicalContext.Facts...))
	baseView := ApplyIncidentView(base, visiblePolicyFor(base.CanonicalContext.Facts...))

	query := ListQuery{Statuses: []Status{StatusOpen}, ProjectStoreID: listScope.StoreID, ProjectID: listScope.ProjectID, Environment: listScope.Environment, CheckID: "stuck-invoices"}
	require.NoError(t, query.Validate())
	require.True(t, MatchesListQuery(recurrenceView, query))
	require.False(t, MatchesListQuery(baseView, query))
	require.False(t, MatchesListQuery(unrelatedView, query))

	hits := Search([]IncidentView{unrelatedView, recurrenceView, baseView}, SearchQuery{Text: "invoice", Facts: []FactSignal{{Entity: "Customer", Field: "Email", Value: investigation.NewStringValue("a@b.com")}}})
	require.Equal(t, []IncidentRef{base.Ref, recurrence.Ref}, []IncidentRef{hits[0].Incident.Ref, hits[1].Incident.Ref})
	require.NotEmpty(t, hits[0].MatchedSignals)
	factOnly := Search([]IncidentView{baseView}, SearchQuery{Facts: []FactSignal{{Entity: "Customer", Field: "Email", Value: investigation.NewStringValue("a@b.com")}}})
	require.Len(t, factOnly, 1)
	redactedView := ApplyIncidentView(base, ViewPolicy{Facts: map[investigation.FactKey]FactVisibility{base.CanonicalContext.Facts[0].Key(): FactValueRedacted}})
	require.Empty(t, Search([]IncidentView{redactedView}, SearchQuery{Facts: []FactSignal{{Entity: "Customer", Field: "Email", Value: investigation.NewStringValue("a@b.com")}}}))

	matches := Similar(recurrenceView, []IncidentView{unrelatedView, baseView, recurrenceView})
	require.Equal(t, base.Ref, matches[0].Incident.Ref)
	require.Greater(t, matches[0].Score, matches[1].Score)
	require.NotEmpty(t, matches[0].MatchedSignals)
}

func TestListFiltersBindArtifactsToFullProjectScope(t *testing.T) {
	requested := ProjectRef{StoreID: "store-a", ProjectID: "project-a", Environment: "prod"}
	wrongScopes := []ProjectRef{
		{StoreID: "store-b", ProjectID: requested.ProjectID, Environment: requested.Environment},
		{StoreID: requested.StoreID, ProjectID: requested.ProjectID, Environment: "staging"},
		{StoreID: requested.StoreID, ProjectID: "project-b", Environment: requested.Environment},
	}
	queryFor := func(kind RefKind) ListQuery {
		query := ListQuery{ProjectStoreID: requested.StoreID, ProjectID: requested.ProjectID, Environment: requested.Environment}
		switch kind {
		case RefQuery:
			query.QueryID = "shared-id"
		case RefCheck:
			query.CheckID = "shared-id"
		case RefBoard:
			query.BoardID = "shared-id"
		}
		return query
	}
	viewFor := func(projects []ProjectRef, kind RefKind, artifactScope ProjectRef) IncidentView {
		incident := task3Projection("INC-10", "Scoped filter", StatusOpen)
		incident.Projects = projects
		incident.AssetRefs = []ArtifactRef{{Kind: kind, Artifact: &ProjectArtifactRef{
			StoreID: artifactScope.StoreID, ProjectID: artifactScope.ProjectID,
			Environment: artifactScope.Environment, ID: "shared-id",
		}}}
		return ApplyIncidentView(incident, visiblePolicyFor(incident.CanonicalContext.Facts...))
	}

	for _, wrong := range wrongScopes {
		require.False(t, MatchesListQuery(viewFor([]ProjectRef{wrong}, RefQuery, wrong), ListQuery{
			ProjectStoreID: requested.StoreID, ProjectID: requested.ProjectID, Environment: requested.Environment,
		}), "project membership must include store and environment")
		for _, kind := range []RefKind{RefQuery, RefCheck, RefBoard} {
			view := viewFor([]ProjectRef{requested, wrong}, kind, wrong)
			require.False(t, MatchesListQuery(view, queryFor(kind)), "%s ref under %+v must not match %+v", kind, wrong, requested)
			require.True(t, MatchesListQuery(viewFor([]ProjectRef{requested, wrong}, kind, requested), queryFor(kind)))
		}
	}

	require.Error(t, (ListQuery{ProjectID: requested.ProjectID, QueryID: "shared-id"}).Validate())
	require.False(t, MatchesListQuery(viewFor([]ProjectRef{requested}, RefQuery, requested), ListQuery{ProjectID: requested.ProjectID, QueryID: "shared-id"}))
	require.Error(t, (ListQuery{QueryID: "shared-id"}).Validate())
	require.False(t, MatchesListQuery(viewFor([]ProjectRef{requested}, RefQuery, requested), ListQuery{QueryID: "shared-id"}))
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
	var _ APIStore = (*task3StubStore)(nil)
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
func (*task3StubStore) List(context.Context, CandidateListQuery) ([]Incident, error) { return nil, nil }
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
		Reporter:       Actor{Kind: ActorHuman, ID: "alex", Via: ActorViaAPI},
		PrimaryProject: ProjectRef{StoreID: "ops", ProjectID: "billing", Environment: "prod"},
		Projects:       []ProjectRef{{StoreID: "ops", ProjectID: "billing", Environment: "prod"}},
		CanonicalContext: investigation.Context{Facts: []investigation.Fact{{
			ID: "customer-email", Entity: "Customer", Field: "Email",
			Value: investigation.NewStringValue("a@b.com"), Origin: investigation.FactOriginManual,
			Enabled: true, Layer: "canonical",
			Scope:    &investigation.ProjectScope{StoreID: "ops", ProjectID: "billing", Environment: "prod"},
			Physical: &investigation.PhysicalRef{Source: "chinook", Collection: "Customer", Column: "Email"},
		}}},
	}
}

func visiblePolicyFor(facts ...investigation.Fact) ViewPolicy {
	decisions := make(map[investigation.FactKey]FactVisibility, len(facts))
	for _, fact := range facts {
		decisions[fact.Key()] = FactVisible
	}
	return ViewPolicy{Facts: decisions}
}

func cloneFacts(facts []investigation.Fact) []investigation.Fact {
	cloned := make([]investigation.Fact, len(facts))
	for index, fact := range facts {
		cloned[index] = fact
		if fact.Scope != nil {
			scope := *fact.Scope
			cloned[index].Scope = &scope
		}
		if fact.Physical != nil {
			physical := *fact.Physical
			cloned[index].Physical = &physical
		}
	}
	return cloned
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

func allArtifactRefShapes() []ArtifactRef {
	return []ArtifactRef{
		{Kind: RefIncident, Incident: &IncidentRef{StoreID: "ops", IncidentID: "INC-9"}},
		{Kind: RefProject, Project: &ProjectRef{StoreID: "ops", ProjectID: "billing", Environment: "prod"}},
		{Kind: RefExecution, Execution: &ExecutionRef{StoreID: "ops", ProjectID: "billing", ExecutionID: "execution-1"}},
		{Kind: RefQuery, Artifact: &ProjectArtifactRef{StoreID: "ops", ProjectID: "billing", Environment: "prod", ID: "query-1"}},
		{Kind: RefCompare, Comparison: &ComparisonRef{
			Left:  ExecutionRef{StoreID: "ops", ProjectID: "billing", ExecutionID: "left-1"},
			Right: ExecutionRef{StoreID: "ops", ProjectID: "billing", ExecutionID: "right-1"},
		}},
	}
}

func mutateAllArtifactRefs(refs []ArtifactRef) {
	refs[0].Incident.IncidentID = "mutated"
	refs[1].Project.ProjectID = "mutated"
	refs[2].Execution.ExecutionID = "mutated"
	refs[3].Artifact.ID = "mutated"
	refs[4].Comparison.Left.ExecutionID = "mutated"
}

func requireAllArtifactRefsUnchanged(t *testing.T, refs []ArtifactRef) {
	t.Helper()
	require.Equal(t, allArtifactRefShapes(), refs)
}

func mustTask3JSON(value any) json.RawMessage {
	b, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return b
}
