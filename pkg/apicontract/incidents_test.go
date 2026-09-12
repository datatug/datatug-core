package apicontract

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/incidents"
	"github.com/stretchr/testify/require"
)

func validIncidentScope() IncidentScope {
	return IncidentScope{StoreID: "ops", Project: "billing", Environment: "prod", SecurityContextID: "ctx-1"}
}

func TestIncidentScopeAndCreateRequestValidate(t *testing.T) {
	valid := IncidentCreateRequest{
		IncidentScope: validIncidentScope(), MutationID: "mutation-create-1", Title: "Invoices stuck",
		Projects: []incidents.ProjectRef{{StoreID: "ops", ProjectID: "billing"}},
	}
	require.NoError(t, valid.Validate())

	missingStore := valid
	missingStore.StoreID = ""
	require.Error(t, missingStore.Validate())
	badScope := valid
	badScope.SecurityContextID = ""
	require.Error(t, badScope.Validate())
	badMutation := valid
	badMutation.MutationID = "../escape"
	require.Error(t, badMutation.Validate())
	missingTitle := valid
	missingTitle.Title = ""
	require.Error(t, missingTitle.Validate())
	badProject := valid
	badProject.Projects[0].ProjectID = ""
	require.Error(t, badProject.Validate())
}

func TestIncidentAppendRequestValidate(t *testing.T) {
	valid := validAppendRequest()
	require.NoError(t, valid.Validate())

	badScope := valid
	badScope.Project = ""
	require.Error(t, badScope.Validate())
	badMutation := valid
	badMutation.MutationID = ""
	require.Error(t, badMutation.Validate())
	badIncident := valid
	badIncident.Incident = incidents.IncidentRef{}
	require.Error(t, badIncident.Validate())
	wrongStore := valid
	wrongStore.StoreID = "other"
	require.Error(t, wrongStore.Validate())
	badEvent := valid
	badEvent.Event.Payload = nil
	require.Error(t, badEvent.Validate())
	for _, eventType := range []incidents.EventType{incidents.EventIncidentCreated, incidents.EventIncidentMerged} {
		dedicatedEndpoint := valid
		dedicatedEndpoint.Event.Type = eventType
		require.Error(t, dedicatedEndpoint.Validate(), eventType)
	}
}

func TestIncidentMergeRequestValidate(t *testing.T) {
	valid := IncidentMergeRequest{
		IncidentScope: validIncidentScope(), MutationID: "mutation-merge-1",
		Source: incidents.IncidentRef{StoreID: "ops", IncidentID: "INC-2"},
		Into:   incidents.IncidentRef{StoreID: "ops", IncidentID: "INC-1"},
	}
	require.NoError(t, valid.Validate())
	badScope := valid
	badScope.Environment = ""
	require.Error(t, badScope.Validate())
	badMutation := valid
	badMutation.Source = badMutation.Into
	require.Error(t, badMutation.Validate())
	wrongStore := valid
	wrongStore.StoreID = "other"
	require.Error(t, wrongStore.Validate())
}

func TestIncidentResponseValidation(t *testing.T) {
	projection := validIncidentProjection("INC-1")
	require.NoError(t, (IncidentResponse{Incident: projection}).Validate())
	require.NoError(t, (IncidentListResponse{Incidents: []incidents.Incident{projection}}).Validate())

	for name, mutate := range map[string]func(*incidents.Incident){
		"ref":        func(v *incidents.Incident) { v.Ref = incidents.IncidentRef{} },
		"uid":        func(v *incidents.Incident) { v.UID = "" },
		"title":      func(v *incidents.Incident) { v.Title = "" },
		"lastSeq":    func(v *incidents.Incident) { v.LastSeq = 0 },
		"status":     func(v *incidents.Incident) { v.Status = "paused" },
		"outcome":    func(v *incidents.Incident) { v.Outcome = "duplicate" },
		"mergedInto": func(v *incidents.Incident) { v.MergedInto = &incidents.IncidentRef{} },
		"selfMerge":  func(v *incidents.Incident) { merged := v.Ref; v.MergedInto = &merged },
		"project":    func(v *incidents.Incident) { v.Projects = []incidents.ProjectRef{{StoreID: "ops"}} },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := projection
			mutate(&invalid)
			require.Error(t, (IncidentResponse{Incident: invalid}).Validate())
			require.Error(t, (IncidentListResponse{Incidents: []incidents.Incident{invalid}}).Validate())
		})
	}

	for _, status := range []incidents.Status{
		incidents.StatusOpen, incidents.StatusInvestigating, incidents.StatusMitigating,
		incidents.StatusRecovering, incidents.StatusResolved, incidents.StatusWatching, incidents.StatusClosed,
	} {
		candidate := projection
		candidate.Status = status
		require.NoError(t, (IncidentResponse{Incident: candidate}).Validate(), status)
	}
	for _, outcome := range []incidents.Outcome{
		incidents.OutcomeResolved, incidents.OutcomeFalseAlarm, incidents.OutcomeAccepted,
		incidents.OutcomeHandedOff, incidents.OutcomeUnresolved,
	} {
		candidate := projection
		candidate.Outcome = outcome
		require.NoError(t, (IncidentResponse{Incident: candidate}).Validate(), outcome)
	}
}

func TestIncidentAppendResponseValidation(t *testing.T) {
	projection := validIncidentProjection("INC-1")
	projection.LastSeq = 2
	projection.Notes = []string{"checked retry queue"}
	event := validNoteEvent(projection.Ref)
	require.NoError(t, (IncidentAppendResponse{Event: event, Projection: projection}).Validate())

	badEvent := event
	badEvent.Seq = 0
	require.Error(t, (IncidentAppendResponse{Event: badEvent, Projection: projection}).Validate())
	badProjection := projection
	badProjection.Title = ""
	require.Error(t, (IncidentAppendResponse{Event: event, Projection: badProjection}).Validate())
	wrongIncident := projection
	wrongIncident.Ref.IncidentID = "INC-9"
	require.Error(t, (IncidentAppendResponse{Event: event, Projection: wrongIncident}).Validate())
	wrongSequence := projection
	wrongSequence.LastSeq = 3
	require.Error(t, (IncidentAppendResponse{Event: event, Projection: wrongSequence}).Validate())
}

func TestIncidentMergeResponseValidation(t *testing.T) {
	source := validIncidentProjection("INC-2")
	into := validIncidentProjection("INC-1")
	source.Status = incidents.StatusClosed
	source.LastSeq = 3
	source.MergedInto = &into.Ref
	into.LastSeq = 4
	valid := IncidentMergeResponse{Source: source, Into: into}
	require.NoError(t, valid.Validate())

	badSource := source
	badSource.Title = ""
	require.Error(t, (IncidentMergeResponse{Source: badSource, Into: into}).Validate())
	badInto := into
	badInto.Title = ""
	require.Error(t, (IncidentMergeResponse{Source: source, Into: badInto}).Validate())
	require.Error(t, (IncidentMergeResponse{Source: into, Into: into}).Validate())
	crossStore := into
	crossStore.Ref.StoreID = "other"
	require.Error(t, (IncidentMergeResponse{Source: source, Into: crossStore}).Validate())
	notClosed := source
	notClosed.Status = incidents.StatusOpen
	require.Error(t, (IncidentMergeResponse{Source: notClosed, Into: into}).Validate())
	wrongDestination := source
	other := incidents.IncidentRef{StoreID: "ops", IncidentID: "INC-9"}
	wrongDestination.MergedInto = &other
	require.Error(t, (IncidentMergeResponse{Source: wrongDestination, Into: into}).Validate())
}

func validAppendRequest() IncidentAppendRequest {
	seq := uint64(1)
	return IncidentAppendRequest{
		IncidentScope: validIncidentScope(), MutationID: "mutation-note-1",
		Incident: incidents.IncidentRef{StoreID: "ops", IncidentID: "INC-1"}, ExpectedSeq: &seq,
		Event: IncidentEventInput{
			At: time.Date(2026, 9, 12, 10, 1, 0, 0, time.UTC), Type: incidents.EventNoteAdded,
			Assertion: incidents.Assertion{Kind: incidents.AssertionClaim},
			Payload:   json.RawMessage(`{"body":"checked retry queue"}`),
		},
	}
}

func validIncidentProjection(id string) incidents.Incident {
	return incidents.Incident{
		Ref: incidents.IncidentRef{StoreID: "ops", IncidentID: id},
		UID: "6ddfe079-d5fe-4d77-b322-11b9a31a9766", Title: "Invoices stuck",
		Description: "Billing queue is not draining", Status: incidents.StatusOpen, LastSeq: 1,
		Projects: []incidents.ProjectRef{{StoreID: "ops", ProjectID: "billing"}},
	}
}

func validNoteEvent(ref incidents.IncidentRef) incidents.Event {
	at := time.Date(2026, 9, 12, 10, 1, 0, 0, time.UTC)
	return incidents.Event{
		ID: "evt-2", Seq: 2, At: at, VisibleAt: at, Incident: ref,
		Actor: incidents.Actor{Kind: incidents.ActorHuman, ID: "alex", Via: incidents.ActorViaAPI},
		Type:  incidents.EventNoteAdded, Assertion: incidents.Assertion{Kind: incidents.AssertionClaim},
		Payload: json.RawMessage(`{"body":"checked retry queue"}`),
	}
}
