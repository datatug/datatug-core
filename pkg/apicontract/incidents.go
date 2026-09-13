package apicontract

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/datatug/datatug-core/pkg/incidents"
)

// IncidentScope is carried by every incident mutation. StoreID routes the
// incident/evidence store; Project and Environment identify the source project
// whose store the server resolves independently.
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
	MutationID       string                     `json:"mutationId"`
	Title            string                     `json:"title"`
	Description      string                     `json:"description,omitempty"`
	Projects         []incidents.ProjectRef     `json:"projects,omitempty"`
	CanonicalContext incidents.CanonicalContext `json:"canonicalContext"`
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
	if err := r.CanonicalContext.ValidateScoped(); err != nil {
		return &ValidationError{Field: "canonicalContext", Message: err.Error()}
	}
	return nil
}

// ValidateResolvedProject binds new fact provenance to the primary project
// scope resolved by the server. IncidentScope.StoreID remains only the route
// to the incident/evidence store and is not used as project provenance.
func (r IncidentCreateRequest) ValidateResolvedProject(resolved incidents.ProjectRef) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if resolved.ProjectID != r.Project || resolved.Environment != r.Environment {
		return &ValidationError{Field: "resolvedProject", Message: "must match the requested project and environment"}
	}
	if err := validateCanonicalContextInput(r.CanonicalContext, resolved, r.Projects); err != nil {
		return &ValidationError{Field: "canonicalContext", Message: err.Error()}
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

// validateStrictNestedJSON lets DecodeStrict validate the type selected by an
// event discriminator even though Payload is retained as json.RawMessage for
// append-only storage.
func (r *IncidentAppendRequest) validateStrictNestedJSON() error {
	switch r.Event.Type {
	case incidents.EventContextFactAdded:
		var payload incidents.ContextFactAddedPayload
		return DecodeStrict(r.Event.Payload, &payload)
	case incidents.EventContextFactPromoted:
		var payload incidents.ContextFactPromotedPayload
		return DecodeStrict(r.Event.Payload, &payload)
	case incidents.EventContextFactRejected:
		var payload incidents.ContextFactRejectedPayload
		return DecodeStrict(r.Event.Payload, &payload)
	default:
		return nil
	}
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
	Incident incidents.IncidentView `json:"incident"`
}

func (r IncidentResponse) Validate() error { return validateIncidentProjection(r.Incident) }

// IncidentListResponse is GET /datatug/incidents.
type IncidentListResponse struct {
	Incidents []incidents.IncidentView `json:"incidents"`
}

func (r IncidentListResponse) Validate() error {
	for i, incident := range r.Incidents {
		if err := validateIncidentProjection(incident); err != nil {
			return &ValidationError{Field: "incidents", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return nil
}

// IncidentAppendResponse returns policy-filtered views of the committed event
// and projection. The provider-facing AppendResult remains canonical storage.
type IncidentAppendResponse struct {
	Event      incidents.Event        `json:"event"`
	Projection incidents.IncidentView `json:"projection"`
	Replayed   bool                   `json:"replayed"`
}

func (r IncidentAppendResponse) Validate() error {
	if err := r.Event.ValidateView(); err != nil {
		return &ValidationError{Field: "event", Message: err.Error()}
	}
	if err := validateIncidentProjection(r.Projection); err != nil {
		return &ValidationError{Field: "projection", Message: err.Error()}
	}
	if r.Event.Incident != r.Projection.Ref {
		return &ValidationError{Field: "projection", Message: "must match event.incident"}
	}
	if r.Event.Seq != r.Projection.LastSeq {
		return &ValidationError{Field: "projection.lastSeq", Message: "must match event.seq"}
	}
	return nil
}

// IncidentMergeResponse returns current-policy projections after a recoverable
// merge; MergeResult remains the provider-facing canonical result.
type IncidentMergeResponse struct {
	Source   incidents.IncidentView `json:"source"`
	Into     incidents.IncidentView `json:"into"`
	Replayed bool                   `json:"replayed"`
}

func (r IncidentMergeResponse) Validate() error {
	if err := validateIncidentProjection(r.Source); err != nil {
		return &ValidationError{Field: "source", Message: err.Error()}
	}
	if err := validateIncidentProjection(r.Into); err != nil {
		return &ValidationError{Field: "into", Message: err.Error()}
	}
	if r.Source.Ref == r.Into.Ref || r.Source.Ref.StoreID != r.Into.Ref.StoreID {
		return &ValidationError{Field: "source", Message: "must differ from into within the same store"}
	}
	if r.Source.Status != incidents.StatusClosed || r.Source.MergedInto == nil || *r.Source.MergedInto != r.Into.Ref {
		return &ValidationError{Field: "source", Message: "must be closed and merged into the destination"}
	}
	return nil
}

func validateIncidentProjection(incident incidents.IncidentView) error {
	return incident.Validate()
}

func validateCanonicalContextInput(context incidents.CanonicalContext, primary incidents.ProjectRef, declared []incidents.ProjectRef) error {
	return context.ValidateAllowedScopes(primary, declared)
}

// IncidentListRequest is GET /datatug/incidents. Scope routes the incident
// store and identifies the source project for server resolution; the remaining
// fields are derived event back-links.
type IncidentListRequest struct {
	IncidentScope
	Statuses []incidents.Status `json:"statuses,omitempty"`
	QueryID  string             `json:"query,omitempty"`
	CheckID  string             `json:"check,omitempty"`
	BoardID  string             `json:"board,omitempty"`
}

func (r IncidentListRequest) Validate() error {
	if err := r.IncidentScope.Validate(); err != nil {
		return err
	}
	if err := (incidents.CandidateListQuery{Statuses: r.Statuses}).Validate(); err != nil {
		return err
	}
	return (incidents.ListQuery{QueryID: r.QueryID, CheckID: r.CheckID, BoardID: r.BoardID}).ValidateArtifactFilters()
}

// ListQuery validates and carries the server-resolved project scope into both
// provider candidate selection and current-policy backlink filtering.
// IncidentScope's StoreID remains solely the incident/evidence-store route.
func (r IncidentListRequest) ListQuery(resolved incidents.ProjectRef) (incidents.ListQuery, error) {
	if err := r.Validate(); err != nil {
		return incidents.ListQuery{}, err
	}
	if err := resolved.ValidateFactScope(); err != nil {
		return incidents.ListQuery{}, &ValidationError{Field: "resolvedProject", Message: err.Error()}
	}
	if resolved.ProjectID != r.Project || resolved.Environment != r.Environment {
		return incidents.ListQuery{}, &ValidationError{Field: "resolvedProject", Message: "must match the requested project and environment"}
	}
	query := incidents.ListQuery{
		Statuses: r.Statuses, ProjectStoreID: resolved.StoreID, ProjectID: resolved.ProjectID, Environment: resolved.Environment,
		QueryID: r.QueryID, CheckID: r.CheckID, BoardID: r.BoardID,
	}
	return query, nil
}

type IncidentSearchRequest struct {
	IncidentScope
	Text  string                 `json:"text"`
	Facts []incidents.FactSignal `json:"facts,omitempty"`
}

func (r IncidentSearchRequest) Validate() error {
	if err := r.IncidentScope.Validate(); err != nil {
		return err
	}
	return (incidents.SearchQuery{Text: r.Text, Facts: r.Facts}).Validate()
}

type IncidentSearchResponse struct {
	Matches []incidents.SearchMatch `json:"matches"`
}

func (r IncidentSearchResponse) Validate() error {
	for i, match := range r.Matches {
		if err := match.Validate(); err != nil {
			return &ValidationError{Field: "matches", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return nil
}

type IncidentSimilarRequest struct {
	IncidentScope
	Incident incidents.IncidentRef `json:"incident"`
}

func (r IncidentSimilarRequest) Validate() error {
	if err := r.IncidentScope.Validate(); err != nil {
		return err
	}
	if err := r.Incident.Validate(); err != nil {
		return &ValidationError{Field: "incident", Message: err.Error()}
	}
	if r.StoreID != r.Incident.StoreID {
		return &ValidationError{Field: "storeId", Message: "must match incident.storeId"}
	}
	return nil
}

type IncidentSimilarResponse struct {
	Matches []incidents.SimilarMatch `json:"matches"`
}

func (r IncidentSimilarResponse) Validate() error {
	previousScore := int(^uint(0) >> 1)
	for i, match := range r.Matches {
		if err := match.Validate(); err != nil {
			return &ValidationError{Field: "matches", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		if match.Score > previousScore {
			return &ValidationError{Field: "matches", Message: "must be ordered by descending score"}
		}
		previousScore = match.Score
	}
	return nil
}

// IncidentEventsRequest selects either a single incident stream or the whole
// current store. Since is opaque to clients and interpreted only by the store.
type IncidentEventsRequest struct {
	IncidentScope
	Incident *incidents.IncidentRef `json:"incident,omitempty"`
	Since    incidents.EventCursor  `json:"since,omitempty"`
}

func (r IncidentEventsRequest) Validate() error {
	if err := r.IncidentScope.Validate(); err != nil {
		return err
	}
	query := incidents.WatchQuery{Incident: r.Incident, Since: r.Since}
	if err := query.Validate(); err != nil {
		return err
	}
	if r.Incident != nil && r.StoreID != r.Incident.StoreID {
		return &ValidationError{Field: "storeId", Message: "must match incident.storeId"}
	}
	return nil
}

type IncidentStreamItem = incidents.StreamItem
