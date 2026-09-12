package apicontract

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/datatug/datatug-core/pkg/incidents"
)

// IncidentScope is carried by every incident mutation. StoreID selects the
// incident store; the remaining fields bind the current project context.
type IncidentScope Scope

func (s IncidentScope) Validate() error {
	scope := Scope(s)
	if err := requireNonEmpty("storeId", scope.StoreID); err != nil {
		return err
	}
	return scope.Validate()
}

// IncidentCreateRequest is POST /datatug/incidents. The server allocates the
// INC-n identifier and derives the reporter from the authenticated principal.
type IncidentCreateRequest struct {
	IncidentScope
	MutationID  string                 `json:"mutationId"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Projects    []incidents.ProjectRef `json:"projects,omitempty"`
}

func (r IncidentCreateRequest) Validate() error {
	if err := r.IncidentScope.Validate(); err != nil {
		return err
	}
	if err := incidents.ValidateMutationID(r.MutationID); err != nil {
		return &ValidationError{Field: "mutationId", Message: err.Error()}
	}
	if err := requireNonEmpty("title", r.Title); err != nil {
		return err
	}
	for i, project := range r.Projects {
		if err := project.Validate(); err != nil {
			return &ValidationError{Field: "projects", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return nil
}

// IncidentEventInput is the caller-controlled portion of an event. Actor and
// visibleAt are absent: the server attests both from its trusted context.
type IncidentEventInput struct {
	At        time.Time               `json:"at"`
	Type      incidents.EventType     `json:"type"`
	Assertion incidents.Assertion     `json:"assertion"`
	Refs      []incidents.ArtifactRef `json:"refs,omitempty"`
	Payload   json.RawMessage         `json:"payload"`
}

func (i IncidentEventInput) validate(incident incidents.IncidentRef) error {
	if i.Type == incidents.EventIncidentCreated || i.Type == incidents.EventIncidentMerged {
		return fmt.Errorf("event type %q requires its dedicated endpoint", i.Type)
	}
	// This pass validates caller-controlled structure only. The handler must
	// rebuild and validate the final Event with the server-attested actor so
	// agent-specific evidence requirements cannot be bypassed.
	draft := incidents.EventDraft{
		At: i.At, Actor: incidents.Actor{Kind: incidents.ActorSystem, ID: "transport"},
		Type: i.Type, Assertion: i.Assertion, Refs: i.Refs, Payload: i.Payload,
	}
	return draft.Validate(incident)
}

// IncidentAppendRequest is POST /datatug/incidents/{id}/events.
type IncidentAppendRequest struct {
	IncidentScope
	MutationID  string                `json:"mutationId"`
	Incident    incidents.IncidentRef `json:"incident"`
	ExpectedSeq *uint64               `json:"expectedSeq,omitempty"`
	Event       IncidentEventInput    `json:"event"`
}

func (r IncidentAppendRequest) Validate() error {
	if err := r.IncidentScope.Validate(); err != nil {
		return err
	}
	if err := incidents.ValidateMutationID(r.MutationID); err != nil {
		return &ValidationError{Field: "mutationId", Message: err.Error()}
	}
	if err := r.Incident.Validate(); err != nil {
		return &ValidationError{Field: "incident", Message: err.Error()}
	}
	if r.StoreID != r.Incident.StoreID {
		return &ValidationError{Field: "storeId", Message: "must match incident.storeId"}
	}
	if err := r.Event.validate(r.Incident); err != nil {
		return &ValidationError{Field: "event", Message: err.Error()}
	}
	return nil
}

// IncidentMergeRequest is POST /datatug/incidents/{id}/merge.
type IncidentMergeRequest struct {
	IncidentScope
	MutationID string                `json:"mutationId"`
	Source     incidents.IncidentRef `json:"source"`
	Into       incidents.IncidentRef `json:"into"`
}

func (r IncidentMergeRequest) Validate() error {
	if err := r.IncidentScope.Validate(); err != nil {
		return err
	}
	mutation := incidents.MergeMutation{MutationID: r.MutationID, Source: r.Source, Into: r.Into}
	if err := mutation.Validate(); err != nil {
		return err
	}
	if r.StoreID != r.Source.StoreID {
		return &ValidationError{Field: "storeId", Message: "must match source.storeId and into.storeId"}
	}
	return nil
}

// IncidentResponse is the success envelope shared by create and show.
type IncidentResponse struct {
	Incident incidents.Incident `json:"incident"`
}

func (r IncidentResponse) Validate() error { return validateIncidentProjection(r.Incident) }

// IncidentListResponse is GET /datatug/incidents.
type IncidentListResponse struct {
	Incidents []incidents.Incident `json:"incidents"`
}

func (r IncidentListResponse) Validate() error {
	for i, incident := range r.Incidents {
		if err := validateIncidentProjection(incident); err != nil {
			return &ValidationError{Field: "incidents", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return nil
}

// IncidentAppendResponse returns the committed event and projection.
type IncidentAppendResponse incidents.AppendResult

func (r IncidentAppendResponse) Validate() error {
	result := incidents.AppendResult(r)
	if err := result.Event.Validate(); err != nil {
		return &ValidationError{Field: "event", Message: err.Error()}
	}
	if err := validateIncidentProjection(result.Projection); err != nil {
		return &ValidationError{Field: "projection", Message: err.Error()}
	}
	if result.Event.Incident != result.Projection.Ref {
		return &ValidationError{Field: "projection", Message: "must match event.incident"}
	}
	if result.Event.Seq != result.Projection.LastSeq {
		return &ValidationError{Field: "projection.lastSeq", Message: "must match event.seq"}
	}
	return nil
}

// IncidentMergeResponse returns both projections after a recoverable merge.
type IncidentMergeResponse incidents.MergeResult

func (r IncidentMergeResponse) Validate() error {
	result := incidents.MergeResult(r)
	if err := validateIncidentProjection(result.Source); err != nil {
		return &ValidationError{Field: "source", Message: err.Error()}
	}
	if err := validateIncidentProjection(result.Into); err != nil {
		return &ValidationError{Field: "into", Message: err.Error()}
	}
	if result.Source.Ref == result.Into.Ref || result.Source.Ref.StoreID != result.Into.Ref.StoreID {
		return &ValidationError{Field: "source", Message: "must differ from into within the same store"}
	}
	if result.Source.Status != incidents.StatusClosed || result.Source.MergedInto == nil || *result.Source.MergedInto != result.Into.Ref {
		return &ValidationError{Field: "source", Message: "must be closed and merged into the destination"}
	}
	return nil
}

func validateIncidentProjection(incident incidents.Incident) error {
	if err := incident.Ref.Validate(); err != nil {
		return err
	}
	if err := requireNonEmpty("uid", incident.UID); err != nil {
		return err
	}
	if err := requireNonEmpty("title", incident.Title); err != nil {
		return err
	}
	if incident.LastSeq == 0 {
		return &ValidationError{Field: "lastSeq", Message: "must be positive"}
	}
	switch incident.Status {
	case incidents.StatusOpen, incidents.StatusInvestigating, incidents.StatusMitigating,
		incidents.StatusRecovering, incidents.StatusResolved, incidents.StatusWatching, incidents.StatusClosed:
	default:
		return &ValidationError{Field: "status", Message: fmt.Sprintf("invalid value %q", incident.Status)}
	}
	if incident.Outcome != "" {
		switch incident.Outcome {
		case incidents.OutcomeResolved, incidents.OutcomeFalseAlarm, incidents.OutcomeAccepted,
			incidents.OutcomeHandedOff, incidents.OutcomeUnresolved:
		default:
			return &ValidationError{Field: "outcome", Message: fmt.Sprintf("invalid value %q", incident.Outcome)}
		}
	}
	if incident.MergedInto != nil {
		if err := incident.MergedInto.Validate(); err != nil {
			return &ValidationError{Field: "mergedInto", Message: err.Error()}
		}
		if *incident.MergedInto == incident.Ref {
			return &ValidationError{Field: "mergedInto", Message: "must identify a different incident"}
		}
	}
	for i, project := range incident.Projects {
		if err := project.Validate(); err != nil {
			return &ValidationError{Field: "projects", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return nil
}
