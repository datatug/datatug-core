package incidents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

func Fold(events []Event, at *time.Time) (Incident, error) {
	var projection Incident
	var expectedSeq uint64 = 1
	for _, event := range events {
		if at != nil && event.VisibleAt.After(*at) {
			continue
		}
		if event.Seq != expectedSeq {
			return Incident{}, fmt.Errorf("incidents: expected sequence %d, got %d", expectedSeq, event.Seq)
		}
		expectedSeq++
		if err := event.Validate(); err != nil {
			return Incident{}, fmt.Errorf("incidents: event %d: %w", event.Seq, err)
		}
		if event.Seq == 1 {
			if event.Type != EventIncidentCreated {
				return Incident{}, fmt.Errorf("incidents: first event must be incident.created")
			}
			projection.Ref = event.Incident
		} else if event.Incident != projection.Ref {
			return Incident{}, fmt.Errorf("incidents: event incident %s does not match %s", event.Incident, projection.Ref)
		}
		if err := projection.apply(event); err != nil {
			return Incident{}, fmt.Errorf("incidents: event %d: %w", event.Seq, err)
		}
		projection.LastSeq = event.Seq
	}
	if projection.LastSeq == 0 {
		return Incident{}, fmt.Errorf("incidents: no visible events")
	}
	return projection, nil
}

func (e Event) Validate() error {
	if strings.TrimSpace(e.ID) == "" || e.Seq == 0 {
		return fmt.Errorf("id and positive seq are required")
	}
	if e.At.IsZero() || e.VisibleAt.IsZero() {
		return fmt.Errorf("at and visibleAt are required")
	}
	if err := e.Incident.Validate(); err != nil {
		return err
	}
	if e.Actor.Kind != ActorHuman && e.Actor.Kind != ActorAgent && e.Actor.Kind != ActorSystem {
		return fmt.Errorf("invalid actor kind %q", e.Actor.Kind)
	}
	if strings.TrimSpace(e.Actor.ID) == "" {
		return fmt.Errorf("actor id is required")
	}
	if e.Actor.Via != "" && e.Actor.Via != ActorViaWeb && e.Actor.Via != ActorViaCLI && e.Actor.Via != ActorViaAPI && e.Actor.Via != ActorViaSlack {
		return fmt.Errorf("invalid actor via %q", e.Actor.Via)
	}
	if !validAssertionKind(e.Assertion.Kind) {
		return fmt.Errorf("invalid assertion kind %q", e.Assertion.Kind)
	}
	if e.Assertion.Confidence != "" && e.Assertion.Confidence != ConfidenceSpeculative && e.Assertion.Confidence != ConfidenceLikely && e.Assertion.Confidence != ConfidenceConfirmed {
		return fmt.Errorf("invalid assertion confidence %q", e.Assertion.Confidence)
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
	return e.validatePayload()
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
	if strings.TrimSpace(r.StoreID) == "" || strings.TrimSpace(r.StoreID) != r.StoreID || strings.Contains(r.StoreID, "/") {
		return fmt.Errorf("invalid execution storeId")
	}
	if strings.TrimSpace(r.ProjectID) == "" || strings.TrimSpace(r.ProjectID) != r.ProjectID || strings.TrimSpace(r.ExecutionID) == "" || strings.TrimSpace(r.ExecutionID) != r.ExecutionID {
		return fmt.Errorf("execution projectId and executionId are required")
	}
	return nil
}

func (r ProjectArtifactRef) Validate() error {
	if err := (ProjectRef{StoreID: r.StoreID, ProjectID: r.ProjectID, Environment: r.Environment}).Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.ID) != r.ID {
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
		p.Notes = append(p.Notes, payload.Body)
	}
	return nil
}

func validatedPayload[T any](data json.RawMessage) T {
	var payload T
	_ = json.Unmarshal(data, &payload) // Event.Validate already decoded this exact payload.
	return payload
}

func decodePayload(data json.RawMessage, dst any) error {
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

func (e Event) validatePayload() error {
	switch e.Type {
	case EventIncidentCreated:
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
	case EventNoteAdded:
		var payload NoteAddedPayload
		if err := decodePayload(e.Payload, &payload); err != nil {
			return err
		}
		if strings.TrimSpace(payload.Body) == "" {
			return fmt.Errorf("note body is required")
		}
	default:
		return fmt.Errorf("unsupported event type %q", e.Type)
	}
	return nil
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
