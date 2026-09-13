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

func TestIncidentListSearchSimilarAndStreamContracts(t *testing.T) {
	scope := validIncidentScope()
	list := IncidentListRequest{IncidentScope: scope, Statuses: []incidents.Status{incidents.StatusOpen}, QueryID: "customers/invoices"}
	require.NoError(t, list.Validate())
	require.Equal(t, incidents.ListQuery{
		Statuses: list.Statuses, StoreID: scope.StoreID, ProjectID: scope.Project,
		Environment: scope.Environment, QueryID: list.QueryID,
	}, list.ListQuery())
	encodedList, err := json.Marshal(list)
	require.NoError(t, err)
	require.JSONEq(t, `{"storeId":"ops","project":"billing","environment":"prod","securityContextId":"ctx-1","statuses":["open"],"query":"customers/invoices"}`, string(encodedList))
	list.CheckID, list.BoardID = "stuck-invoices", "incident-board"
	require.NoError(t, list.Validate())
	list.Statuses = []incidents.Status{"paused"}
	require.Error(t, list.Validate())

	search := IncidentSearchRequest{IncidentScope: scope, Text: "invoice", Facts: []incidents.FactSignal{{
		Entity: "Customer", Field: "ID", Value: investigation.NewIntegerValue("5"),
	}}}
	require.NoError(t, search.Validate())
	search.Text = ""
	search.Facts = nil
	require.Error(t, search.Validate())

	projection := validIncidentView("INC-1")
	searchResponse := IncidentSearchResponse{Matches: []incidents.SearchMatch{{Incident: projection, MatchedSignals: []incidents.MatchedSignal{{Kind: incidents.SignalText, Value: "invoice"}}}}}
	require.NoError(t, searchResponse.Validate())
	similarResponse := IncidentSimilarResponse{Matches: []incidents.SimilarMatch{{Incident: projection, Score: 3, MatchedSignals: []incidents.MatchedSignal{{Kind: incidents.SignalFact, Value: "Customer.ID=integer:5"}}}}}
	require.NoError(t, similarResponse.Validate())

	ref := incidents.IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	events := IncidentEventsRequest{IncidentScope: scope, Incident: &ref, Since: "cursor-1"}
	require.NoError(t, events.Validate())
	events.Since = "bad cursor"
	require.Error(t, events.Validate())

	item := IncidentStreamItem{Cursor: "cursor-2", Event: validNoteEvent(ref)}
	require.NoError(t, item.Validate())
}

func TestIncidentTask3RequestAndResponseValidationFailures(t *testing.T) {
	scope := validIncidentScope()
	badScope := scope
	badScope.Project = ""

	require.Error(t, (IncidentListRequest{IncidentScope: badScope}).Validate())
	require.Error(t, (IncidentSearchRequest{IncidentScope: badScope, Text: "invoice"}).Validate())

	view := validIncidentView("INC-1")
	require.Error(t, (IncidentSearchResponse{Matches: []incidents.SearchMatch{{Incident: incidents.IncidentView{}, MatchedSignals: []incidents.MatchedSignal{{Kind: incidents.SignalText, Value: "invoice"}}}}}).Validate())

	ref := incidents.IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	require.NoError(t, (IncidentSimilarRequest{IncidentScope: scope, Incident: ref}).Validate())
	require.Error(t, (IncidentSimilarRequest{IncidentScope: badScope, Incident: ref}).Validate())
	require.Error(t, (IncidentSimilarRequest{IncidentScope: scope, Incident: incidents.IncidentRef{}}).Validate())
	wrongStore := ref
	wrongStore.StoreID = "other"
	require.Error(t, (IncidentSimilarRequest{IncidentScope: scope, Incident: wrongStore}).Validate())

	signal := []incidents.MatchedSignal{{Kind: incidents.SignalText, Value: "invoice"}}
	require.Error(t, (IncidentSimilarResponse{Matches: []incidents.SimilarMatch{{Incident: incidents.IncidentView{}, Score: 1, MatchedSignals: signal}}}).Validate())
	require.Error(t, (IncidentSimilarResponse{Matches: []incidents.SimilarMatch{
		{Incident: view, Score: 1, MatchedSignals: signal},
		{Incident: view, Score: 2, MatchedSignals: signal},
	}}).Validate())

	require.Error(t, (IncidentEventsRequest{IncidentScope: badScope}).Validate())
	require.Error(t, (IncidentEventsRequest{IncidentScope: scope, Since: "bad cursor"}).Validate())
	require.Error(t, (IncidentEventsRequest{IncidentScope: scope, Incident: &wrongStore}).Validate())
	require.NoError(t, (IncidentEventsRequest{IncidentScope: scope}).Validate())

	create := IncidentCreateRequest{IncidentScope: scope, MutationID: "create-1", Title: "Invoice alert"}
	create.CanonicalContext.Facts = []investigation.Fact{{}}
	require.Error(t, create.Validate())
	create.CanonicalContext.Facts = []investigation.Fact{{
		ID: "customer-id", Entity: "Customer", Field: "ID", Value: NewIntegerValue("5"),
		Origin: FactOriginContext, Enabled: true,
	}}
	require.Error(t, create.Validate())
}

func TestIncidentContextUsesCanonicalAPIContractFact(t *testing.T) {
	scope := ProjectScope{StoreID: "ops", ProjectID: "billing", Environment: "prod"}
	acceptIncidentScope := func(incidents.ProjectRef) {}
	acceptIncidentScope(scope)
	canonical := Fact{
		ID: "customer-id", Entity: "Customer", Field: "ID", Value: NewIntegerValue("5"),
		Origin: FactOriginContext, Enabled: true, Role: FactRoleAffected, Layer: "canonical", Scope: &scope,
	}
	incident := incidents.Incident{CanonicalContext: incidents.CanonicalContext{Facts: []investigation.Fact{canonical}}}
	apiJSON, err := json.Marshal(canonical)
	require.NoError(t, err)
	incidentJSON, err := json.Marshal(incident.CanonicalContext.Facts[0])
	require.NoError(t, err)
	require.JSONEq(t, string(apiJSON), string(incidentJSON))

	var roundTrip Fact
	require.NoError(t, json.Unmarshal(incidentJSON, &roundTrip))
	require.Equal(t, canonical, roundTrip)
}

func TestIncidentCreateBindsFactScopesToRequestProjects(t *testing.T) {
	primary := validIncidentScope()
	primaryScope := investigation.ProjectScope{StoreID: primary.StoreID, ProjectID: primary.Project, Environment: primary.Environment}
	secondary := incidents.ProjectRef{StoreID: "warehouse", ProjectID: "payments", Environment: "staging"}
	fact := investigation.Fact{
		ID: "customer-id", Entity: "Customer", Field: "ID", Value: NewIntegerValue("5"),
		Origin: FactOriginManual, Enabled: true, Scope: &primaryScope,
	}
	request := IncidentCreateRequest{IncidentScope: primary, MutationID: "create-1", Title: "Invoice issue", Projects: []incidents.ProjectRef{secondary}}
	request.CanonicalContext.Facts = []investigation.Fact{fact}
	require.NoError(t, request.Validate())
	secondaryFact := fact
	secondaryFact.Scope = &secondary
	request.CanonicalContext.Facts = append(request.CanonicalContext.Facts, secondaryFact)
	require.NoError(t, request.Validate())

	for _, mutate := range []func(*IncidentCreateRequest){
		func(r *IncidentCreateRequest) { r.CanonicalContext.Facts[0].Scope.StoreID = "foreign" },
		func(r *IncidentCreateRequest) { r.CanonicalContext.Facts[0].Scope.ProjectID = "undeclared" },
		func(r *IncidentCreateRequest) { r.CanonicalContext.Facts[0].Scope.Environment = "staging" },
	} {
		candidate := request
		candidate.CanonicalContext.Facts = append([]investigation.Fact(nil), request.CanonicalContext.Facts...)
		scope := *candidate.CanonicalContext.Facts[0].Scope
		candidate.CanonicalContext.Facts[0].Scope = &scope
		mutate(&candidate)
		require.Error(t, candidate.Validate())
	}
}

func TestIncidentReadEnvelopesCarryRedactionWithoutPhysicalMapping(t *testing.T) {
	stored := validIncidentProjection("INC-1")
	stored.CanonicalContext = investigation.Context{Facts: []investigation.Fact{{
		ID: "customer-email", Entity: "Customer", Field: "Email",
		Value: investigation.NewStringValue("secret@example.com"), Origin: investigation.FactOriginContext,
		Physical: &investigation.PhysicalRef{Source: "crm", Collection: "Customer", Column: "Email"},
		Enabled:  true, Layer: "canonical",
	}}}
	view := incidents.ApplyIncidentView(stored, incidents.ViewPolicy{Facts: map[investigation.FactKey]incidents.FactVisibility{
		stored.CanonicalContext.Facts[0].Key(): incidents.FactValueRedacted,
	}})
	redactedEvent := redactedCreatedEvent(t, view)

	source := view
	source.Ref.IncidentID = "INC-2"
	source.Status = incidents.StatusClosed
	source.MergedInto = &view.Ref

	envelopes := []struct {
		name  string
		value interface {
			Validate() error
		}
	}{
		{"show", IncidentResponse{Incident: view}},
		{"list", IncidentListResponse{Incidents: []incidents.IncidentView{view}}},
		{"search", IncidentSearchResponse{Matches: []incidents.SearchMatch{{Incident: view, MatchedSignals: []incidents.MatchedSignal{{Kind: incidents.SignalText, Value: "invoice"}}}}}},
		{"similar", IncidentSimilarResponse{Matches: []incidents.SimilarMatch{{Incident: view, Score: 1, MatchedSignals: []incidents.MatchedSignal{{Kind: incidents.SignalProject, Value: "ops/billing/"}}}}}},
		{"append", IncidentAppendResponse{Event: redactedEvent, Projection: view}},
		{"merge", IncidentMergeResponse{Source: source, Into: view}},
	}
	for _, envelope := range envelopes {
		t.Run(envelope.name, func(t *testing.T) {
			require.NoError(t, envelope.value.Validate())
			data, err := json.Marshal(envelope.value)
			require.NoError(t, err)
			require.Contains(t, string(data), `"value":{"redacted":true}`)
			require.NotContains(t, string(data), `"physical"`)
			require.NotContains(t, string(data), "secret@example.com")
		})
	}

	stream := incidents.StreamItem{Cursor: "cursor-1", Event: redactedEvent}
	require.NoError(t, stream.Validate())
	data, err := json.Marshal(stream)
	require.NoError(t, err)
	require.Contains(t, string(data), `"value":{"redacted":true}`)
	require.False(t, strings.Contains(string(data), `"physical"`))
}

func redactedCreatedEvent(t *testing.T, view incidents.IncidentView) incidents.Event {
	t.Helper()
	at := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(incidents.CreatedViewPayload{
		UID: view.UID, Title: view.Title, Description: view.Description, Projects: view.Projects,
		Reporter:         incidents.Actor{Kind: incidents.ActorHuman, ID: "alex", Via: incidents.ActorViaAPI},
		CanonicalContext: view.CanonicalContext,
	})
	require.NoError(t, err)
	return incidents.Event{
		ID: "create-1", Seq: view.LastSeq, At: at, VisibleAt: at, Incident: view.Ref,
		Actor: incidents.Actor{Kind: incidents.ActorHuman, ID: "alex", Via: incidents.ActorViaAPI},
		Type:  incidents.EventIncidentCreated, Assertion: incidents.Assertion{Kind: incidents.AssertionClaim},
		Payload: payload,
	}
}
