package incidents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/datatug/datatug-core/internal/jsonstrict"
	"github.com/datatug/datatug-core/pkg/investigation"
)

func Fold(events []Event, at *time.Time) (Incident, error) {
	if err := validateEventStream(events); err != nil {
		return Incident{}, err
	}
	var projection Incident
	for _, event := range events {
		if at != nil && event.VisibleAt.After(*at) {
			continue
		}
		if event.Seq == 1 {
			projection.Ref = event.Incident
		} else if event.Incident != projection.Ref {
			return Incident{}, fmt.Errorf("incidents: event incident %s does not match %s", event.Incident, projection.Ref)
		}
		if event.ImportedFrom == nil {
			if err := projection.apply(event); err != nil {
				return Incident{}, fmt.Errorf("incidents: event %d: %w", event.Seq, err)
			}
		}
		projection.LastSeq = event.Seq
	}
	if projection.LastSeq == 0 {
		return Incident{}, fmt.Errorf("incidents: no visible events")
	}
	return projection, nil
}

func validateEventStream(events []Event) error {
	var previousVisibleAt time.Time
	var activeMergeID string
	var activeMergeVisibleAt time.Time
	var activeMergeSource IncidentRef
	var activeSourceSeq uint64
	completedMerges := make(map[string]struct{})
	for i, event := range events {
		expectedSeq := uint64(i + 1)
		if event.Seq != expectedSeq {
			return fmt.Errorf("incidents: expected sequence %d, got %d", expectedSeq, event.Seq)
		}
		if err := event.Validate(); err != nil {
			return fmt.Errorf("incidents: event %d: %w", event.Seq, err)
		}
		if event.Seq == 1 && event.Type != EventIncidentCreated {
			return fmt.Errorf("incidents: first event must be incident.created")
		}
		if !previousVisibleAt.IsZero() && event.VisibleAt.Before(previousVisibleAt) {
			return fmt.Errorf("incidents: event %d visibleAt precedes prior event", event.Seq)
		}
		previousVisibleAt = event.VisibleAt

		if event.ImportedFrom == nil {
			if activeMergeID != "" {
				completedMerges[activeMergeID] = struct{}{}
				activeMergeID = ""
			}
			continue
		}
		mergeID := event.ImportedFrom.MergeID
		if activeMergeID == "" {
			if _, completed := completedMerges[mergeID]; completed {
				return fmt.Errorf("incidents: imported merge %q is not contiguous", mergeID)
			}
			activeMergeID = mergeID
			activeMergeVisibleAt = event.VisibleAt
			activeMergeSource = event.ImportedFrom.Incident
			activeSourceSeq = event.ImportedFrom.Seq
			if activeSourceSeq != 1 {
				return fmt.Errorf("incidents: imported merge %q must start at source sequence 1", mergeID)
			}
			continue
		}
		if mergeID != activeMergeID {
			completedMerges[activeMergeID] = struct{}{}
			if _, completed := completedMerges[mergeID]; completed {
				return fmt.Errorf("incidents: imported merge %q is not contiguous", mergeID)
			}
			activeMergeID = mergeID
			activeMergeVisibleAt = event.VisibleAt
			activeMergeSource = event.ImportedFrom.Incident
			activeSourceSeq = event.ImportedFrom.Seq
			if activeSourceSeq != 1 {
				return fmt.Errorf("incidents: imported merge %q must start at source sequence 1", mergeID)
			}
			continue
		}
		if !event.VisibleAt.Equal(activeMergeVisibleAt) {
			return fmt.Errorf("incidents: imported merge %q has split visibility", mergeID)
		}
		if event.ImportedFrom.Incident != activeMergeSource {
			return fmt.Errorf("incidents: imported merge %q mixes source incidents", mergeID)
		}
		if event.ImportedFrom.Seq != activeSourceSeq+1 {
			return fmt.Errorf("incidents: imported merge %q source sequence is not contiguous", mergeID)
		}
		activeSourceSeq = event.ImportedFrom.Seq
	}
	return nil
}

func (e Event) Validate() error {
	return e.validate(false)
}

// ValidateView validates a detached, policy-filtered event. Unlike persisted
// events, a view may contain explicit fact-value redaction markers.
func (e Event) ValidateView() error {
	return e.validate(true)
}

func (e Event) validate(view bool) error {
	if strings.TrimSpace(e.ID) == "" || e.Seq == 0 {
		return fmt.Errorf("id and positive seq are required")
	}
	if e.At.IsZero() || e.VisibleAt.IsZero() {
		return fmt.Errorf("at and visibleAt are required")
	}
	if err := e.Incident.Validate(); err != nil {
		return err
	}
	if e.ImportedFrom != nil {
		if e.Seq == 1 {
			return fmt.Errorf("first event cannot be imported")
		}
		if err := e.ImportedFrom.Validate(e.Incident); err != nil {
			return err
		}
	}
	if err := e.Actor.Validate(); err != nil {
		return err
	}
	if !validAssertionKind(e.Assertion.Kind) {
		return fmt.Errorf("invalid assertion kind %q", e.Assertion.Kind)
	}
	if e.Assertion.Confidence != "" && e.Assertion.Confidence != ConfidenceSpeculative && e.Assertion.Confidence != ConfidenceLikely && e.Assertion.Confidence != ConfidenceConfirmed {
		return fmt.Errorf("invalid assertion confidence %q", e.Assertion.Confidence)
	}
	if e.Type == EventCompareRun {
		if e.Assertion.Kind != AssertionDeterministicResult {
			return fmt.Errorf("compare.run requires deterministic-result assertion")
		}
		if len(e.Refs) != 1 || e.Refs[0].Kind != RefCompare {
			return fmt.Errorf("compare.run requires exactly one compare reference")
		}
	}
	if e.Assertion.Kind == AssertionInference && !hasEventRef(e.Refs) {
		return fmt.Errorf("inference requires at least one event reference")
	}
	if e.Actor.Kind == ActorAgent && (e.Assertion.Kind == AssertionObservation || e.Assertion.Kind == AssertionDeterministicResult) && !hasEvidenceRef(e.Refs) {
		return fmt.Errorf("agent %s requires an execution, check, or compare reference", e.Assertion.Kind)
	}
	for i, ref := range e.Refs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("ref %d: %w", i, err)
		}
	}
	if len(e.Payload) == 0 {
		return fmt.Errorf("payload is required")
	}
	return e.validatePayload(view)
}

func (r ArtifactRef) Validate() error {
	set := 0
	if r.ID != "" {
		set++
	}
	if r.Incident != nil {
		set++
	}
	if r.Project != nil {
		set++
	}
	if r.Execution != nil {
		set++
	}
	if r.Artifact != nil {
		set++
	}
	if r.Comparison != nil {
		set++
	}
	if set != 1 {
		return fmt.Errorf("exactly one reference identity is required")
	}
	switch r.Kind {
	case RefIncident:
		if r.Incident == nil {
			return fmt.Errorf("incident ref requires IncidentRef")
		}
		return r.Incident.Validate()
	case RefProject:
		if r.Project == nil {
			return fmt.Errorf("project ref requires ProjectRef")
		}
		return r.Project.Validate()
	case RefExecution, RefSnapshot:
		if r.Execution == nil {
			return fmt.Errorf("execution ref requires ExecutionRef")
		}
		return r.Execution.Validate()
	case RefEvent, RefHypothesis:
		if !validSegment(r.ID) {
			return fmt.Errorf("%s ref id is required and canonical", r.Kind)
		}
		return nil
	case RefFact, RefAnnotation, RefCheck, RefQuery, RefBoard:
		if r.Artifact == nil {
			return fmt.Errorf("%s ref requires ProjectArtifactRef", r.Kind)
		}
		return r.Artifact.Validate()
	case RefCompare:
		if r.Comparison == nil {
			return fmt.Errorf("compare ref requires ComparisonRef")
		}
		return r.Comparison.Validate()
	default:
		return fmt.Errorf("invalid ref kind %q", r.Kind)
	}
}

func (r ExecutionRef) Validate() error {
	if !validSegment(r.StoreID) {
		return fmt.Errorf("invalid execution storeId")
	}
	if !validSegment(r.ProjectID) || !validSegment(r.ExecutionID) {
		return fmt.Errorf("execution projectId and executionId are required")
	}
	return nil
}

func (r ProjectArtifactRef) Validate() error {
	if err := (ProjectRef{StoreID: r.StoreID, ProjectID: r.ProjectID, Environment: r.Environment}).Validate(); err != nil {
		return err
	}
	if !validSegment(r.ID) {
		return fmt.Errorf("artifact id is required and canonical")
	}
	return nil
}

func (r ComparisonRef) Validate() error {
	if err := r.Left.Validate(); err != nil {
		return fmt.Errorf("left: %w", err)
	}
	if err := r.Right.Validate(); err != nil {
		return fmt.Errorf("right: %w", err)
	}
	return nil
}

func (p *Incident) apply(event Event) error {
	if p.MergedInto != nil {
		return fmt.Errorf("merged incident is terminal")
	}
	switch event.Type {
	case EventIncidentCreated:
		if p.LastSeq != 0 {
			return fmt.Errorf("incident.created may occur only once")
		}
		payload := validatedPayload[CreatedPayload](event.Payload)
		p.UID, p.Title, p.Description, p.Projects = payload.UID, payload.Title, payload.Description, payload.Projects
		if payload.Reporter.ID != "" {
			p.Participants = []Participant{{Actor: payload.Reporter, Role: ParticipantReporter}}
		}
		p.CanonicalContext = payload.CanonicalContext
		p.Status = StatusOpen
	case EventIncidentStatus:
		payload := validatedPayload[StatusPayload](event.Payload)
		if statusRank(payload.Status) < statusRank(p.Status) {
			return fmt.Errorf("status cannot move backward from %s to %s", p.Status, payload.Status)
		}
		if (payload.Status == StatusWatching || payload.Status == StatusClosed) && p.Outcome == "" {
			return fmt.Errorf("status %s requires an outcome", payload.Status)
		}
		p.Status = payload.Status
	case EventIncidentOutcome:
		payload := validatedPayload[OutcomePayload](event.Payload)
		if p.Status != StatusResolved {
			return fmt.Errorf("outcome requires resolved status")
		}
		if p.Outcome != "" {
			return fmt.Errorf("outcome is already recorded")
		}
		p.Outcome = payload.Outcome
	case EventIncidentMerged:
		payload := validatedPayload[MergedPayload](event.Payload)
		p.MergedInto = &payload.Into
		p.Status = StatusClosed
		p.Outcome = ""
	case EventNoteAdded:
		payload := validatedPayload[NoteAddedPayload](event.Payload)
		p.NoteEntries = append(p.NoteEntries, Note{EventID: event.ID, Body: payload.Body})
		p.Notes = append(p.Notes, payload.Body)
	case EventContextFactAdded:
		payload := validatedPayload[ContextFactAddedPayload](event.Payload)
		layer := investigation.NormalizeFactLayer(payload.Fact.Layer)
		if contextLayerRejected(p.ContextRejections, layer) {
			return fmt.Errorf("cannot add a fact to rejected overlay %q", layer)
		}
		ref := ContextFactRef{Scope: *payload.Fact.Scope, ID: payload.Fact.ID, Layer: layer}
		if contextFactIndex(p.CanonicalContext.Facts, ref) >= 0 {
			return fmt.Errorf("context fact %q already exists in layer %q", payload.Fact.ID, layer)
		}
		p.CanonicalContext.Facts = append(p.CanonicalContext.Facts, cloneContextFact(payload.Fact))
	case EventContextFactPromoted:
		payload := validatedPayload[ContextFactPromotedPayload](event.Payload)
		if contextLayerRejected(p.ContextRejections, payload.Fact.Layer) {
			return fmt.Errorf("cannot promote from rejected overlay %q", payload.Fact.Layer)
		}
		index := contextFactIndex(p.CanonicalContext.Facts, payload.Fact)
		if index < 0 {
			return fmt.Errorf("overlay fact %q not found in layer %q", payload.Fact.ID, payload.Fact.Layer)
		}
		canonicalRef := payload.Fact
		canonicalRef.Layer = investigation.FactLayerCanonical
		if contextFactIndex(p.CanonicalContext.Facts, canonicalRef) >= 0 {
			return fmt.Errorf("canonical fact %q already exists", payload.Fact.ID)
		}
		canonical := cloneContextFact(p.CanonicalContext.Facts[index])
		canonical.Layer = investigation.FactLayerCanonical
		canonical.Role = payload.Role
		p.CanonicalContext.Facts = append(p.CanonicalContext.Facts, canonical)
		p.ContextPromotions = append(p.ContextPromotions, ContextPromotion{EventID: event.ID, Fact: payload.Fact, Role: payload.Role})
	case EventContextFactRejected:
		payload := validatedPayload[ContextFactRejectedPayload](event.Payload)
		if contextLayerRejected(p.ContextRejections, payload.Layer) {
			return fmt.Errorf("overlay %q is already rejected", payload.Layer)
		}
		if contextLayerPromoted(p.ContextPromotions, payload.Layer) {
			return fmt.Errorf("cannot reject promoted overlay %q", payload.Layer)
		}
		if !contextLayerHasFacts(p.CanonicalContext.Facts, payload.Layer) {
			return fmt.Errorf("overlay %q has no facts", payload.Layer)
		}
		p.ContextRejections = append(p.ContextRejections, ContextRejection{EventID: event.ID, Layer: payload.Layer})
	case EventCompareRun:
		// The qualified comparison reference is projected below. Diff rows are
		// intentionally absent from the durable event payload.
	}
	for _, ref := range event.Refs {
		if ref.Kind == RefQuery || ref.Kind == RefCheck || ref.Kind == RefBoard || ref.Kind == RefCompare {
			p.AssetRefEntries = appendUniqueAssetRefEntry(p.AssetRefEntries, AssetRefEntry{EventID: event.ID, Ref: ref})
			p.AssetRefs = appendUniqueArtifactRef(p.AssetRefs, ref)
		}
	}
	return nil
}

func validatedPayload[T any](data json.RawMessage) T {
	var payload T
	_ = json.Unmarshal(data, &payload) // Event.Validate already decoded this exact payload.
	return payload
}

func decodePayload(data json.RawMessage, dst any) error {
	if err := jsonstrict.CheckNoDuplicateKeysFor(data, reflect.TypeOf(dst)); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("invalid payload: trailing data")
	}
	return nil
}

func (e Event) validatePayload(view bool) error {
	switch e.Type {
	case EventIncidentCreated:
		if view {
			var payload CreatedViewPayload
			if err := decodePayload(e.Payload, &payload); err != nil {
				return err
			}
			if strings.TrimSpace(payload.UID) == "" || strings.TrimSpace(payload.Title) == "" {
				return fmt.Errorf("created payload requires uid and title")
			}
			for i, project := range payload.Projects {
				if err := project.Validate(); err != nil {
					return fmt.Errorf("project %d: %w", i, err)
				}
			}
			if payload.Reporter.ID != "" {
				if err := payload.Reporter.Validate(); err != nil {
					return fmt.Errorf("reporter: %w", err)
				}
			}
			if err := payload.CanonicalContext.Validate(); err != nil {
				return fmt.Errorf("canonicalContext: %w", err)
			}
			return nil
		}
		var payload CreatedPayload
		if err := decodePayload(e.Payload, &payload); err != nil {
			return err
		}
		if strings.TrimSpace(payload.UID) == "" || strings.TrimSpace(payload.Title) == "" {
			return fmt.Errorf("created payload uid and title are required")
		}
		for i, project := range payload.Projects {
			if err := project.Validate(); err != nil {
				return fmt.Errorf("project %d: %w", i, err)
			}
		}
		if payload.Reporter.ID != "" {
			if err := payload.Reporter.Validate(); err != nil {
				return fmt.Errorf("reporter: %w", err)
			}
		}
		if err := payload.CanonicalContext.Validate(); err != nil {
			return fmt.Errorf("canonicalContext: %w", err)
		}
	case EventIncidentStatus:
		var payload StatusPayload
		if err := decodePayload(e.Payload, &payload); err != nil {
			return err
		}
		if !validStatus(payload.Status) {
			return fmt.Errorf("invalid status %q", payload.Status)
		}
	case EventIncidentOutcome:
		var payload OutcomePayload
		if err := decodePayload(e.Payload, &payload); err != nil {
			return err
		}
		if !validOutcome(payload.Outcome) {
			return fmt.Errorf("invalid outcome %q", payload.Outcome)
		}
	case EventIncidentMerged:
		var payload MergedPayload
		if err := decodePayload(e.Payload, &payload); err != nil {
			return err
		}
		if err := payload.Into.Validate(); err != nil {
			return err
		}
		if payload.Into == e.Incident {
			return fmt.Errorf("incident cannot merge into itself")
		}
		if !validSegment(payload.MergeID) {
			return fmt.Errorf("merged payload requires valid mergeId")
		}
	case EventNoteAdded:
		var payload NoteAddedPayload
		if err := decodePayload(e.Payload, &payload); err != nil {
			return err
		}
		if strings.TrimSpace(payload.Body) == "" {
			return fmt.Errorf("note body is required")
		}
	case EventContextFactAdded:
		if view {
			var payload ContextFactAddedViewPayload
			if err := decodePayload(e.Payload, &payload); err != nil {
				return err
			}
			if err := payload.Fact.Validate(); err != nil {
				return fmt.Errorf("fact: %w", err)
			}
			if payload.Fact.Scope == nil {
				return fmt.Errorf("fact scope with environment is required")
			}
			if err := payload.Fact.Scope.ValidateFactScope(); err != nil {
				return fmt.Errorf("fact scope: %w", err)
			}
			return validateContextHypothesisRefs(e.Refs, payload.Fact.Layer)
		}
		var payload ContextFactAddedPayload
		if err := decodePayload(e.Payload, &payload); err != nil {
			return err
		}
		if err := (investigation.Context{Facts: []investigation.Fact{payload.Fact}}).ValidateScoped(); err != nil {
			return fmt.Errorf("fact: %w", err)
		}
		if err := validateContextHypothesisRefs(e.Refs, payload.Fact.Layer); err != nil {
			return err
		}
	case EventContextFactPromoted:
		var payload ContextFactPromotedPayload
		if err := decodePayload(e.Payload, &payload); err != nil {
			return err
		}
		if err := payload.Validate(); err != nil {
			return err
		}
		if err := validateContextHypothesisRefs(e.Refs, payload.Fact.Layer); err != nil {
			return err
		}
	case EventContextFactRejected:
		var payload ContextFactRejectedPayload
		if err := decodePayload(e.Payload, &payload); err != nil {
			return err
		}
		if err := payload.Validate(); err != nil {
			return err
		}
		if err := validateContextHypothesisRefs(e.Refs, payload.Layer); err != nil {
			return err
		}
	case EventCompareRun:
		if e.Assertion.Kind != AssertionDeterministicResult {
			return fmt.Errorf("compare.run requires deterministic-result assertion")
		}
		if len(e.Refs) != 1 || e.Refs[0].Kind != RefCompare {
			return fmt.Errorf("compare.run requires exactly one compare reference")
		}
		var payload CompareRunPayload
		if err := decodePayload(e.Payload, &payload); err != nil {
			return err
		}
		if len(payload.Key) == 0 {
			return fmt.Errorf("compare.run key is required")
		}
		seen := map[string]bool{}
		for _, column := range payload.Key {
			if strings.TrimSpace(column) == "" {
				return fmt.Errorf("compare.run key column is required")
			}
			if seen[column] {
				return fmt.Errorf("compare.run key column %q is duplicated", column)
			}
			seen[column] = true
		}
	default:
		return fmt.Errorf("unsupported event type %q", e.Type)
	}
	return nil
}

func appendUniqueArtifactRef(refs []ArtifactRef, candidate ArtifactRef) []ArtifactRef {
	for _, ref := range refs {
		if artifactRefsEqual(ref, candidate) {
			return refs
		}
	}
	return append(refs, cloneArtifactRef(candidate))
}

func appendUniqueAssetRefEntry(entries []AssetRefEntry, candidate AssetRefEntry) []AssetRefEntry {
	for _, entry := range entries {
		if entry.EventID == candidate.EventID && artifactRefsEqual(entry.Ref, candidate.Ref) {
			return entries
		}
	}
	candidate.Ref = cloneArtifactRef(candidate.Ref)
	return append(entries, candidate)
}

func artifactRefsEqual(left, right ArtifactRef) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return bytes.Equal(a, b)
}

func validAssertionKind(value AssertionKind) bool {
	switch value {
	case AssertionObservation, AssertionClaim, AssertionQuestion, AssertionHypothesis, AssertionInference, AssertionDeterministicResult:
		return true
	default:
		return false
	}
}

func hasEvidenceRef(refs []ArtifactRef) bool {
	for _, ref := range refs {
		if ref.Kind == RefExecution || ref.Kind == RefCheck || ref.Kind == RefCompare {
			return true
		}
	}
	return false
}

func hasEventRef(refs []ArtifactRef) bool {
	for _, ref := range refs {
		if ref.Kind == RefEvent {
			return true
		}
	}
	return false
}

func validStatus(value Status) bool { return statusRank(value) >= 0 }

func statusRank(value Status) int {
	switch value {
	case StatusOpen:
		return 0
	case StatusInvestigating:
		return 1
	case StatusMitigating:
		return 2
	case StatusRecovering:
		return 3
	case StatusResolved:
		return 4
	case StatusWatching:
		return 5
	case StatusClosed:
		return 6
	default:
		return -1
	}
}

func validOutcome(value Outcome) bool {
	switch value {
	case OutcomeResolved, OutcomeFalseAlarm, OutcomeAccepted, OutcomeHandedOff, OutcomeUnresolved:
		return true
	default:
		return false
	}
}
