package incidents

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/investigation"
	"github.com/stretchr/testify/require"
)

func TestContextFactPromotionPreservesOverlayAndShowAt(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	start := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	scope := investigation.ProjectScope{StoreID: "projects", ProjectID: "billing", Environment: "prod"}
	overlay := investigation.Fact{
		ID: "customer-11", Entity: "Customer", Field: "ID", Value: investigation.NewIntegerValue("11"),
		Origin: investigation.FactOriginContext, Enabled: true, Role: investigation.FactRoleSuspected,
		Layer: "hypothesis:H17", Scope: &scope,
		Physical: &investigation.PhysicalRef{Source: "crm", Collection: "Customer", Column: "ID"},
	}
	created := task4CreatedEvent(t, ref, start)
	added := eventWithPayload(t, ref, 2, start.Add(time.Minute), EventContextFactAdded, ContextFactAddedPayload{Fact: overlay})
	promoted := eventWithPayload(t, ref, 3, start.Add(2*time.Minute), EventContextFactPromoted, ContextFactPromotedPayload{
		Fact: ContextFactRef{Scope: scope, ID: overlay.ID, Layer: overlay.Layer},
		Role: investigation.FactRoleAffected,
	})

	beforePromotion := start.Add(time.Minute)
	historical, err := Fold([]Event{created, added, promoted}, &beforePromotion)
	require.NoError(t, err)
	require.Equal(t, []investigation.Fact{overlay}, historical.CanonicalContext.Facts)
	require.Empty(t, historical.ContextPromotions)
	require.Equal(t, uint64(2), historical.LastSeq)

	projection, err := Fold([]Event{created, added, promoted}, nil)
	require.NoError(t, err)
	require.Len(t, projection.CanonicalContext.Facts, 2)
	require.Equal(t, overlay, projection.CanonicalContext.Facts[0])
	canonical := projection.CanonicalContext.Facts[1]
	expectedCanonical := overlay
	expectedCanonical.Layer = investigation.FactLayerCanonical
	expectedCanonical.Role = investigation.FactRoleAffected
	require.Equal(t, expectedCanonical, canonical)
	require.Equal(t, []ContextPromotion{{
		EventID: promoted.ID,
		Fact:    ContextFactRef{Scope: scope, ID: overlay.ID, Layer: overlay.Layer},
		Role:    investigation.FactRoleAffected,
	}}, projection.ContextPromotions)
	require.NoError(t, projection.Validate())
}

func TestContextOverlayRejectionRetainsFactsAndClosesTransitions(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	start := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	scope := investigation.ProjectScope{StoreID: "projects", ProjectID: "billing", Environment: "prod"}
	overlay := investigation.Fact{
		ID: "customer-12", Entity: "Customer", Field: "ID", Value: investigation.NewIntegerValue("12"),
		Origin: investigation.FactOriginContext, Enabled: true, Role: investigation.FactRoleSuspected,
		Layer: "hypothesis:H12", Scope: &scope,
	}
	created := task4CreatedEvent(t, ref, start)
	added := eventWithPayload(t, ref, 2, start.Add(time.Minute), EventContextFactAdded, ContextFactAddedPayload{Fact: overlay})
	rejected := eventWithPayload(t, ref, 3, start.Add(2*time.Minute), EventContextFactRejected, ContextFactRejectedPayload{Layer: overlay.Layer})

	projection, err := Fold([]Event{created, added, rejected}, nil)
	require.NoError(t, err)
	require.Equal(t, []investigation.Fact{overlay}, projection.CanonicalContext.Facts)
	require.Empty(t, projection.ContextPromotions)
	require.Equal(t, []ContextRejection{{EventID: rejected.ID, Layer: overlay.Layer}}, projection.ContextRejections)
	require.NoError(t, projection.Validate())
	beforeRejection := start.Add(time.Minute)
	historical, err := Fold([]Event{created, added, rejected}, &beforeRejection)
	require.NoError(t, err)
	require.Empty(t, historical.ContextRejections)

	addAfterReject := eventWithPayload(t, ref, 4, start.Add(3*time.Minute), EventContextFactAdded, ContextFactAddedPayload{Fact: overlay})
	_, err = Fold([]Event{created, added, rejected, addAfterReject}, nil)
	require.ErrorContains(t, err, "rejected overlay")

	promoteAfterReject := eventWithPayload(t, ref, 4, start.Add(3*time.Minute), EventContextFactPromoted, ContextFactPromotedPayload{
		Fact: ContextFactRef{Scope: scope, ID: overlay.ID, Layer: overlay.Layer}, Role: investigation.FactRoleAffected,
	})
	_, err = Fold([]Event{created, added, rejected, promoteAfterReject}, nil)
	require.ErrorContains(t, err, "rejected overlay")

	rejectAgain := eventWithPayload(t, ref, 4, start.Add(3*time.Minute), EventContextFactRejected, ContextFactRejectedPayload{Layer: overlay.Layer})
	_, err = Fold([]Event{created, added, rejected, rejectAgain}, nil)
	require.ErrorContains(t, err, "already rejected")
}

func TestContextOverlayCannotBeRejectedAfterPromotion(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	start := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	scope := investigation.ProjectScope{StoreID: "projects", ProjectID: "billing", Environment: "prod"}
	overlay := investigation.Fact{
		ID: "customer-12", Entity: "Customer", Field: "ID", Value: investigation.NewIntegerValue("12"),
		Origin: investigation.FactOriginContext, Enabled: true, Role: investigation.FactRoleSuspected,
		Layer: "hypothesis:H12", Scope: &scope,
	}
	created := task4CreatedEvent(t, ref, start)
	added := eventWithPayload(t, ref, 2, start.Add(time.Minute), EventContextFactAdded, ContextFactAddedPayload{Fact: overlay})
	promoted := eventWithPayload(t, ref, 3, start.Add(2*time.Minute), EventContextFactPromoted, ContextFactPromotedPayload{
		Fact: ContextFactRef{Scope: scope, ID: overlay.ID, Layer: overlay.Layer}, Role: investigation.FactRoleAffected,
	})
	rejected := eventWithPayload(t, ref, 4, start.Add(3*time.Minute), EventContextFactRejected, ContextFactRejectedPayload{Layer: overlay.Layer})

	_, err := Fold([]Event{created, added, promoted, rejected}, nil)
	require.ErrorContains(t, err, "promoted overlay")
}

func TestContextEventValidationFailsClosed(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	at := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	scope := investigation.ProjectScope{StoreID: "projects", ProjectID: "billing", Environment: "prod"}
	validFact := investigation.Fact{
		ID: "customer-11", Entity: "Customer", Field: "ID", Value: investigation.NewIntegerValue("11"),
		Origin: investigation.FactOriginContext, Enabled: true, Role: investigation.FactRoleSuspected,
		Layer: "hypothesis:H17", Scope: &scope,
	}
	validRef := ContextFactRef{Scope: scope, ID: validFact.ID, Layer: validFact.Layer}
	tests := []struct {
		name      string
		eventType EventType
		payload   any
	}{
		{"invalid added fact", EventContextFactAdded, ContextFactAddedPayload{Fact: investigation.Fact{Value: investigation.NewStringValue("value")}}},
		{"unscoped added fact", EventContextFactAdded, ContextFactAddedPayload{Fact: func() investigation.Fact { f := validFact; f.Scope = nil; return f }()}},
		{"bad added layer", EventContextFactAdded, ContextFactAddedPayload{Fact: func() investigation.Fact { f := validFact; f.Layer = "hypothesis:"; return f }()}},
		{"missing promotion fact", EventContextFactPromoted, ContextFactPromotedPayload{Role: investigation.FactRoleAffected}},
		{"missing promotion role", EventContextFactPromoted, ContextFactPromotedPayload{Fact: validRef}},
		{"canonical promotion source", EventContextFactPromoted, ContextFactPromotedPayload{Fact: ContextFactRef{Scope: scope, ID: validFact.ID, Layer: investigation.FactLayerCanonical}, Role: investigation.FactRoleAffected}},
		{"bad promotion role", EventContextFactPromoted, ContextFactPromotedPayload{Fact: validRef, Role: "observer"}},
		{"canonical rejection", EventContextFactRejected, ContextFactRejectedPayload{Layer: investigation.FactLayerCanonical}},
		{"empty rejection", EventContextFactRejected, ContextFactRejectedPayload{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := eventWithPayload(t, ref, 2, at, tt.eventType, tt.payload)
			require.Error(t, event.Validate())
		})
	}

	invalidRef := eventWithPayload(t, ref, 2, at, EventContextFactPromoted, ContextFactPromotedPayload{Fact: validRef, Role: investigation.FactRoleAffected})
	invalidRef.Refs = []ArtifactRef{{Kind: RefHypothesis, ID: "bad/ref"}}
	require.Error(t, invalidRef.Validate())
	mismatchedRef := eventWithPayload(t, ref, 2, at, EventContextFactPromoted, ContextFactPromotedPayload{Fact: validRef, Role: investigation.FactRoleAffected})
	mismatchedRef.Refs = []ArtifactRef{{Kind: RefHypothesis, ID: "H12"}}
	require.ErrorContains(t, mismatchedRef.Validate(), "does not match")
	matchingRef := eventWithPayload(t, ref, 2, at, EventContextFactPromoted, ContextFactPromotedPayload{Fact: validRef, Role: investigation.FactRoleAffected})
	matchingRef.Refs = []ArtifactRef{{Kind: RefEvent, ID: "evt-source"}, {Kind: RefHypothesis, ID: "H17"}}
	require.NoError(t, matchingRef.Validate())
	wrongLayerRef := eventWithPayload(t, ref, 2, at, EventContextFactRejected, ContextFactRejectedPayload{Layer: "participant:alex"})
	wrongLayerRef.Refs = []ArtifactRef{{Kind: RefHypothesis, ID: "H17"}}
	require.ErrorContains(t, wrongLayerRef.Validate(), "does not match")
}

func TestContextPayloadAndHistoryValidationFailsClosed(t *testing.T) {
	scope := investigation.ProjectScope{StoreID: "projects", ProjectID: "billing", Environment: "prod"}
	validRef := ContextFactRef{Scope: scope, ID: "customer-11", Layer: "hypothesis:H17"}
	require.NoError(t, validRef.Validate())
	for _, candidate := range []ContextFactRef{
		{ID: validRef.ID, Layer: validRef.Layer},
		{Scope: scope, Layer: validRef.Layer},
		{Scope: scope, ID: validRef.ID, Layer: "hypothesis:"},
		{Scope: scope, ID: validRef.ID, Layer: investigation.FactLayerCanonical},
	} {
		require.Error(t, candidate.Validate())
	}
	require.NoError(t, (ContextFactRejectedPayload{Layer: "participant:alex"}).Validate())
	require.Error(t, (ContextFactRejectedPayload{Layer: "admin"}).Validate())
	require.Error(t, (ContextPromotion{}).Validate())
	require.Error(t, (ContextRejection{}).Validate())
	require.Error(t, (FactSignal{Entity: "Customer", Field: "ID", Value: investigation.NewIntegerValue("11"), Condition: "contains"}).Validate())

	promotedProjection := task4PromotedProjection(t)
	require.NoError(t, promotedProjection.Validate())
	promotionMutations := []func(*Incident){
		func(i *Incident) { i.ContextPromotions[0].EventID = "" },
		func(i *Incident) { i.ContextPromotions = append(i.ContextPromotions, i.ContextPromotions[0]) },
		func(i *Incident) {
			duplicate := i.ContextPromotions[0]
			duplicate.EventID = "evt-5"
			i.ContextPromotions = append(i.ContextPromotions, duplicate)
		},
		func(i *Incident) { i.ContextPromotions[0].Fact.ID = "missing" },
		func(i *Incident) { i.CanonicalContext.Facts = i.CanonicalContext.Facts[:1] },
		func(i *Incident) { i.CanonicalContext.Facts[1].Value = investigation.NewIntegerValue("12") },
		func(i *Incident) {
			i.ContextRejections = []ContextRejection{{EventID: "evt-5", Layer: i.ContextPromotions[0].Fact.Layer}}
		},
	}
	for _, mutate := range promotionMutations {
		candidate := promotedProjection
		candidate.CanonicalContext.Facts = cloneFacts(promotedProjection.CanonicalContext.Facts)
		candidate.ContextPromotions = append([]ContextPromotion(nil), promotedProjection.ContextPromotions...)
		mutate(&candidate)
		require.Error(t, candidate.Validate())
	}

	rejectedProjection := task4RejectedProjection(t)
	require.NoError(t, rejectedProjection.Validate())
	rejectionMutations := []func(*Incident){
		func(i *Incident) { i.ContextRejections[0].EventID = "" },
		func(i *Incident) { i.ContextRejections[0].Layer = "hypothesis:H12" },
		func(i *Incident) {
			i.ContextRejections = append(i.ContextRejections, ContextRejection{EventID: "evt-5", Layer: i.ContextRejections[0].Layer})
		},
	}
	for _, mutate := range rejectionMutations {
		candidate := rejectedProjection
		candidate.CanonicalContext.Facts = cloneFacts(rejectedProjection.CanonicalContext.Facts)
		candidate.ContextRejections = append([]ContextRejection(nil), rejectedProjection.ContextRejections...)
		mutate(&candidate)
		require.Error(t, candidate.Validate())
	}

	view := ApplyIncidentView(promotedProjection, visiblePolicyFor(promotedProjection.CanonicalContext.Facts...))
	require.NoError(t, view.Validate())
	badView := view
	badView.ContextPromotions = []ContextPromotion{{}}
	require.Error(t, badView.Validate())
	badView = view
	badView.ContextPromotions = append(append([]ContextPromotion(nil), view.ContextPromotions...), view.ContextPromotions[0])
	require.Error(t, badView.Validate())
	badView = view
	duplicatePromotion := view.ContextPromotions[0]
	duplicatePromotion.EventID = "evt-5"
	badView.ContextPromotions = append(append([]ContextPromotion(nil), view.ContextPromotions...), duplicatePromotion)
	require.Error(t, badView.Validate())
	badView = view
	badView.ContextRejections = []ContextRejection{{}}
	require.Error(t, badView.Validate())
	badView = view
	badView.ContextRejections = append(append([]ContextRejection(nil), view.ContextRejections...), ContextRejection{EventID: view.ContextPromotions[0].EventID, Layer: "hypothesis:H17"})
	require.Error(t, badView.Validate())
	badView = view
	badView.ContextRejections = []ContextRejection{{EventID: "evt-5", Layer: view.ContextPromotions[0].Fact.Layer}}
	require.Error(t, badView.Validate())

	rejectedView := ApplyIncidentView(rejectedProjection, visiblePolicyFor(rejectedProjection.CanonicalContext.Facts...))
	require.NoError(t, rejectedView.Validate())
	rejectedView.ContextRejections = append(rejectedView.ContextRejections, rejectedView.ContextRejections[0])
	require.Error(t, rejectedView.Validate())
}

func TestContextEventViewValidationAndMalformedPayloads(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	at := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	scope := investigation.ProjectScope{StoreID: "projects", ProjectID: "billing", Environment: "prod"}
	fact := investigation.Fact{
		ID: "customer-11", Entity: "Customer", Field: "ID", Value: investigation.NewIntegerValue("11"),
		Origin: investigation.FactOriginContext, Enabled: true, Layer: "hypothesis:H17", Scope: &scope,
	}
	added := eventWithPayload(t, ref, 2, at, EventContextFactAdded, ContextFactAddedPayload{Fact: fact})
	visible, shown, err := ApplyEventView(added, visiblePolicyFor(fact))
	require.NoError(t, err)
	require.True(t, shown)
	require.NoError(t, visible.ValidateView())
	promotion := eventWithPayload(t, ref, 3, at.Add(time.Minute), EventContextFactPromoted, ContextFactPromotedPayload{
		Fact: ContextFactRef{Scope: scope, ID: fact.ID, Layer: fact.Layer}, Role: investigation.FactRoleAffected,
	})
	_, shown, err = ApplyEventView(promotion, visiblePolicyFor(fact))
	require.NoError(t, err)
	require.True(t, shown)
	_, shown, err = ApplyEventView(promotion, ViewPolicy{})
	require.NoError(t, err)
	require.False(t, shown)

	for _, eventType := range []EventType{EventContextFactAdded, EventContextFactPromoted} {
		malformed := added
		malformed.Type = eventType
		malformed.Payload = json.RawMessage(`{"broken":`)
		_, _, err = ApplyEventView(malformed, ViewPolicy{})
		require.Error(t, err)
	}
	for _, eventType := range []EventType{EventContextFactAdded, EventContextFactPromoted, EventContextFactRejected} {
		malformed := added
		malformed.Type = eventType
		malformed.Payload = json.RawMessage(`{"broken":`)
		require.Error(t, malformed.Validate())
	}
	mismatchedAdd := added
	mismatchedAdd.Refs = []ArtifactRef{{Kind: RefHypothesis, ID: "H12"}}
	require.ErrorContains(t, mismatchedAdd.Validate(), "does not match")

	viewPayload := ContextFactAddedViewPayload{Fact: investigation.VisibleFact(fact)}
	badView := added
	badView.Payload = mustJSON(t, viewPayload)
	viewPayload.Fact.ID = ""
	badView.Payload = mustJSON(t, viewPayload)
	require.Error(t, badView.ValidateView())
	badView.Payload = json.RawMessage(`{"broken":`)
	require.Error(t, badView.ValidateView())
	viewPayload.Fact = investigation.VisibleFact(fact)
	viewPayload.Fact.Scope = nil
	badView.Payload = mustJSON(t, viewPayload)
	require.Error(t, badView.ValidateView())
	viewPayload.Fact = investigation.VisibleFact(fact)
	viewPayload.Fact.Scope.Environment = ""
	badView.Payload = mustJSON(t, viewPayload)
	require.Error(t, badView.ValidateView())

	projection := task4RejectedProjection(t)
	policy := visiblePolicyFor(projection.CanonicalContext.Facts...)
	policy.WithheldEvents = map[string]bool{projection.ContextRejections[0].EventID: true}
	filtered := ApplyIncidentView(projection, policy)
	require.Empty(t, filtered.ContextRejections)
	require.Empty(t, filtered.ContextPromotions)
}

func TestContextEventFoldRejectsMissingOrDuplicateFactsAndLayers(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	start := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	scope := investigation.ProjectScope{StoreID: "projects", ProjectID: "billing", Environment: "prod"}
	overlay := investigation.Fact{
		ID: "customer-11", Entity: "Customer", Field: "ID", Value: investigation.NewIntegerValue("11"),
		Origin: investigation.FactOriginContext, Enabled: true, Role: investigation.FactRoleSuspected,
		Layer: "hypothesis:H17", Scope: &scope,
	}
	created := task4CreatedEvent(t, ref, start)
	added := eventWithPayload(t, ref, 2, start.Add(time.Minute), EventContextFactAdded, ContextFactAddedPayload{Fact: overlay})
	duplicate := eventWithPayload(t, ref, 3, start.Add(2*time.Minute), EventContextFactAdded, ContextFactAddedPayload{Fact: overlay})
	_, err := Fold([]Event{created, added, duplicate}, nil)
	require.ErrorContains(t, err, "already exists")

	missingPromotion := eventWithPayload(t, ref, 2, start.Add(time.Minute), EventContextFactPromoted, ContextFactPromotedPayload{
		Fact: ContextFactRef{Scope: scope, ID: overlay.ID, Layer: overlay.Layer}, Role: investigation.FactRoleAffected,
	})
	_, err = Fold([]Event{created, missingPromotion}, nil)
	require.ErrorContains(t, err, "not found")

	missingRejection := eventWithPayload(t, ref, 2, start.Add(time.Minute), EventContextFactRejected, ContextFactRejectedPayload{Layer: overlay.Layer})
	_, err = Fold([]Event{created, missingRejection}, nil)
	require.ErrorContains(t, err, "has no facts")

	canonical := overlay
	canonical.Layer = investigation.FactLayerCanonical
	createdWithCanonical := task4CreatedEventWithContext(t, ref, start, investigation.Context{Facts: []investigation.Fact{canonical}})
	_, err = Fold([]Event{createdWithCanonical, added, eventWithPayload(t, ref, 3, start.Add(2*time.Minute), EventContextFactPromoted, ContextFactPromotedPayload{
		Fact: ContextFactRef{Scope: scope, ID: overlay.ID, Layer: overlay.Layer}, Role: investigation.FactRoleAffected,
	})}, nil)
	require.ErrorContains(t, err, "canonical fact")
}

func TestContextFactAddedEventAndProjectionViewsArePolicyFiltered(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	start := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	scope := investigation.ProjectScope{StoreID: "projects", ProjectID: "billing", Environment: "prod"}
	fact := investigation.Fact{
		ID: "customer-email", Entity: "Customer", Field: "Email", Value: investigation.NewStringValue("secret@example.com"),
		Origin: investigation.FactOriginContext, Enabled: true, Role: investigation.FactRoleSuspected,
		Layer: "hypothesis:H17", Scope: &scope,
	}
	created := task4CreatedEvent(t, ref, start)
	added := eventWithPayload(t, ref, 2, start.Add(time.Minute), EventContextFactAdded, ContextFactAddedPayload{Fact: fact})
	promoted := eventWithPayload(t, ref, 3, start.Add(2*time.Minute), EventContextFactPromoted, ContextFactPromotedPayload{
		Fact: ContextFactRef{Scope: scope, ID: fact.ID, Layer: fact.Layer}, Role: investigation.FactRoleAffected,
	})
	projection, err := Fold([]Event{created, added, promoted}, nil)
	require.NoError(t, err)

	canonicalFact := fact
	canonicalFact.Layer = investigation.FactLayerCanonical
	redactedPolicy := ViewPolicy{Facts: map[investigation.FactKey]FactVisibility{
		fact.Key():          FactValueRedacted,
		canonicalFact.Key(): FactValueRedacted,
	}}
	eventView, visible, err := ApplyEventView(added, redactedPolicy)
	require.NoError(t, err)
	require.True(t, visible)
	require.NoError(t, eventView.ValidateView())
	var payload ContextFactAddedViewPayload
	require.NoError(t, json.Unmarshal(eventView.Payload, &payload))
	require.True(t, payload.Fact.Redacted())
	wire, err := json.Marshal(eventView)
	require.NoError(t, err)
	require.NotContains(t, string(wire), "secret@example.com")

	projectionView := ApplyIncidentView(projection, redactedPolicy)
	require.Len(t, projectionView.CanonicalContext.Facts, 2)
	require.True(t, projectionView.CanonicalContext.Facts[0].Redacted())
	require.True(t, projectionView.CanonicalContext.Facts[1].Redacted())
	require.Equal(t, projection.ContextPromotions, projectionView.ContextPromotions)
	require.NoError(t, projectionView.Validate())

	_, visible, err = ApplyEventView(added, ViewPolicy{})
	require.NoError(t, err)
	require.False(t, visible)
	hiddenProjection := ApplyIncidentView(projection, ViewPolicy{})
	require.Empty(t, hiddenProjection.CanonicalContext.Facts)
	require.Empty(t, hiddenProjection.ContextPromotions)
}

func task4CreatedEvent(t *testing.T, ref IncidentRef, at time.Time) Event {
	t.Helper()
	return task4CreatedEventWithContext(t, ref, at, investigation.Context{})
}

func task4CreatedEventWithContext(t *testing.T, ref IncidentRef, at time.Time, context investigation.Context) Event {
	t.Helper()
	return eventWithPayload(t, ref, 1, at, EventIncidentCreated, CreatedPayload{UID: "uid-1", Title: "Invoice incident", CanonicalContext: context})
}

func task4PromotedProjection(t *testing.T) Incident {
	t.Helper()
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	start := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	scope := investigation.ProjectScope{StoreID: "projects", ProjectID: "billing", Environment: "prod"}
	fact := investigation.Fact{
		ID: "customer-11", Entity: "Customer", Field: "ID", Value: investigation.NewIntegerValue("11"),
		Origin: investigation.FactOriginContext, Enabled: true, Role: investigation.FactRoleSuspected,
		Layer: "hypothesis:H17", Scope: &scope,
	}
	events := []Event{
		task4CreatedEvent(t, ref, start),
		eventWithPayload(t, ref, 2, start.Add(time.Minute), EventContextFactAdded, ContextFactAddedPayload{Fact: fact}),
		eventWithPayload(t, ref, 3, start.Add(2*time.Minute), EventContextFactPromoted, ContextFactPromotedPayload{
			Fact: ContextFactRef{Scope: scope, ID: fact.ID, Layer: fact.Layer}, Role: investigation.FactRoleAffected,
		}),
	}
	projection, err := Fold(events, nil)
	require.NoError(t, err)
	return projection
}

func task4RejectedProjection(t *testing.T) Incident {
	t.Helper()
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	start := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	scope := investigation.ProjectScope{StoreID: "projects", ProjectID: "billing", Environment: "prod"}
	fact := investigation.Fact{
		ID: "customer-11", Entity: "Customer", Field: "ID", Value: investigation.NewIntegerValue("11"),
		Origin: investigation.FactOriginContext, Enabled: true, Role: investigation.FactRoleSuspected,
		Layer: "hypothesis:H17", Scope: &scope,
	}
	events := []Event{
		task4CreatedEvent(t, ref, start),
		eventWithPayload(t, ref, 2, start.Add(time.Minute), EventContextFactAdded, ContextFactAddedPayload{Fact: fact}),
		eventWithPayload(t, ref, 3, start.Add(2*time.Minute), EventContextFactRejected, ContextFactRejectedPayload{Layer: fact.Layer}),
	}
	projection, err := Fold(events, nil)
	require.NoError(t, err)
	return projection
}
