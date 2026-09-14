// Package incidents defines Incidentius's store-independent incident model and
// deterministic event projection. Persistence and transport adapters live in
// their owning repositories and depend on these validated types.
package incidents

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/datatug/datatug-core/pkg/investigation"
)

// IncidentRef is the globally unambiguous identity of an incident.
type IncidentRef struct {
	StoreID    string `json:"storeId"`
	IncidentID string `json:"incidentId"`
}

func (r IncidentRef) Validate() error {
	if !validSegment(r.StoreID) {
		return fmt.Errorf("incidents: invalid storeId %q", r.StoreID)
	}
	if !validSegment(r.IncidentID) {
		return fmt.Errorf("incidents: invalid incidentId %q", r.IncidentID)
	}
	return nil
}

func (r IncidentRef) String() string {
	return r.StoreID + "/" + r.IncidentID
}

func ParseIncidentRef(value string) (IncidentRef, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return IncidentRef{}, fmt.Errorf("incidents: incident reference must be <store>/<incident>")
	}
	ref := IncidentRef{StoreID: parts[0], IncidentID: parts[1]}
	return ref, ref.Validate()
}

// ProjectRef remains a source- and wire-compatible alias of the one canonical
// persisted project scope used by Investigation Context facts.
type ProjectRef = investigation.ProjectScope

type ActorKind string

const (
	ActorHuman  ActorKind = "human"
	ActorAgent  ActorKind = "agent"
	ActorSystem ActorKind = "system"
)

const (
	ActorViaWeb   = "web"
	ActorViaCLI   = "cli"
	ActorViaAPI   = "api"
	ActorViaSlack = "slack"
)

type Actor struct {
	Kind ActorKind `json:"kind"`
	ID   string    `json:"id"`
	Via  string    `json:"via,omitempty"`
}

type AssertionKind string

const (
	AssertionObservation         AssertionKind = "observation"
	AssertionClaim               AssertionKind = "claim"
	AssertionQuestion            AssertionKind = "question"
	AssertionHypothesis          AssertionKind = "hypothesis"
	AssertionInference           AssertionKind = "inference"
	AssertionDeterministicResult AssertionKind = "deterministic-result"
)

type Assertion struct {
	Kind       AssertionKind `json:"kind"`
	Confidence string        `json:"confidence,omitempty"`
}

const (
	ConfidenceSpeculative = "speculative"
	ConfidenceLikely      = "likely"
	ConfidenceConfirmed   = "confirmed"
)

type RefKind string

const (
	RefExecution  RefKind = "execution"
	RefSnapshot   RefKind = "snapshot"
	RefAnnotation RefKind = "annotation"
	RefCheck      RefKind = "check"
	RefCompare    RefKind = "compare"
	RefQuery      RefKind = "query"
	RefBoard      RefKind = "board"
	RefFact       RefKind = "fact"
	RefHypothesis RefKind = "hypothesis"
	RefEvent      RefKind = "event"
	RefIncident   RefKind = "incident"
	RefProject    RefKind = "project"
)

// ArtifactRef uses structured identities for cross-store artifacts. ID is
// reserved for incident-local hypotheses and events.
type ArtifactRef struct {
	Kind       RefKind             `json:"kind"`
	ID         string              `json:"id,omitempty"`
	Incident   *IncidentRef        `json:"incident,omitempty"`
	Project    *ProjectRef         `json:"project,omitempty"`
	Execution  *ExecutionRef       `json:"execution,omitempty"`
	Artifact   *ProjectArtifactRef `json:"artifact,omitempty"`
	Comparison *ComparisonRef      `json:"comparison,omitempty"`
}

type ExecutionRef struct {
	StoreID     string `json:"storeId"`
	ProjectID   string `json:"projectId"`
	ExecutionID string `json:"executionId"`
}

type ProjectArtifactRef struct {
	StoreID     string `json:"storeId"`
	ProjectID   string `json:"projectId"`
	Environment string `json:"environment,omitempty"`
	ID          string `json:"id"`
}

type ComparisonRef struct {
	Left  ExecutionRef `json:"left"`
	Right ExecutionRef `json:"right"`
}

type EventType string

const (
	EventIncidentCreated     EventType = "incident.created"
	EventIncidentStatus      EventType = "incident.status"
	EventIncidentOutcome     EventType = "incident.outcome"
	EventIncidentMerged      EventType = "incident.merged"
	EventNoteAdded           EventType = "note.added"
	EventContextFactAdded    EventType = "context.fact.added"
	EventContextFactPromoted EventType = "context.fact.promoted"
	EventContextFactRejected EventType = "context.fact.rejected"
	EventCompareRun          EventType = "compare.run"
)

type Event struct {
	ID        string      `json:"id"`
	Seq       uint64      `json:"seq"`
	At        time.Time   `json:"at"`
	VisibleAt time.Time   `json:"visibleAt"`
	Incident  IncidentRef `json:"incident"`
	// ImportedFrom preserves the original identity of an event copied by a
	// merge. Imported events remain timeline evidence but do not mutate the
	// survivor's active projection.
	ImportedFrom *ImportedEventRef `json:"importedFrom,omitempty"`
	Actor        Actor             `json:"actor"`
	Type         EventType         `json:"type"`
	Assertion    Assertion         `json:"assertion"`
	Refs         []ArtifactRef     `json:"refs,omitempty"`
	Payload      json.RawMessage   `json:"payload"`
}

type ImportedEventRef struct {
	Incident IncidentRef `json:"incident"`
	EventID  string      `json:"eventId"`
	Seq      uint64      `json:"seq"`
	MergeID  string      `json:"mergeId"`
}

func (r ImportedEventRef) Validate(destination IncidentRef) error {
	if err := r.Incident.Validate(); err != nil {
		return err
	}
	if r.Incident.StoreID != destination.StoreID {
		return fmt.Errorf("imported event must come from the destination store")
	}
	if r.Incident == destination {
		return fmt.Errorf("imported event must come from a different incident")
	}
	if strings.TrimSpace(r.EventID) == "" || r.Seq == 0 || !validSegment(r.MergeID) {
		return fmt.Errorf("imported event id, positive seq, and valid mergeId are required")
	}
	return nil
}

type Status string

const (
	StatusOpen          Status = "open"
	StatusInvestigating Status = "investigating"
	StatusMitigating    Status = "mitigating"
	StatusRecovering    Status = "recovering"
	StatusResolved      Status = "resolved"
	StatusWatching      Status = "watching"
	StatusClosed        Status = "closed"
)

type Outcome string

const (
	OutcomeResolved   Outcome = "resolved"
	OutcomeFalseAlarm Outcome = "false-alarm"
	OutcomeAccepted   Outcome = "accepted"
	OutcomeHandedOff  Outcome = "handed-off"
	OutcomeUnresolved Outcome = "unresolved"
)

type CreatedPayload struct {
	UID              string           `json:"uid"`
	Title            string           `json:"title"`
	Description      string           `json:"description"`
	Projects         []ProjectRef     `json:"projects,omitempty"`
	Reporter         Actor            `json:"reporter"`
	CanonicalContext CanonicalContext `json:"canonicalContext"`
}

type StatusPayload struct {
	Status Status `json:"status"`
}

type OutcomePayload struct {
	Outcome Outcome `json:"outcome"`
}

type MergedPayload struct {
	Into    IncidentRef `json:"into"`
	MergeID string      `json:"mergeId"`
}

type NoteAddedPayload struct {
	Body string `json:"body"`
}

// CompareRunPayload persists only the deterministic key contract. The row
// diff remains an endpoint result and must never enter the incident event log.
type CompareRunPayload struct {
	Key []string `json:"key"`
}

// Note retains the stable source event identity required to re-check human
// text visibility without keying a policy decision by its sensitive body.
type Note struct {
	EventID string `json:"eventId"`
	Body    string `json:"body"`
}

// AssetRefEntry retains the stable source event for each derived backlink.
// AssetRefs remains the source-compatible unique projection for older readers.
type AssetRefEntry struct {
	EventID string      `json:"eventId"`
	Ref     ArtifactRef `json:"ref"`
}

type ParticipantRole string

const ParticipantReporter ParticipantRole = "reporter"

type Participant struct {
	Actor Actor           `json:"actor"`
	Role  ParticipantRole `json:"role"`
}

// CanonicalContext is an alias, not an incident-specific model. Incidents and
// the transport appendix consume the same Investigation Context type.
type CanonicalContext = investigation.Context

// Incident is the deterministic current or historical projection.
type Incident struct {
	Ref               IncidentRef        `json:"ref"`
	UID               string             `json:"uid"`
	Title             string             `json:"title"`
	Description       string             `json:"description,omitempty"`
	Status            Status             `json:"status"`
	Outcome           Outcome            `json:"outcome,omitempty"`
	MergedInto        *IncidentRef       `json:"mergedInto,omitempty"`
	Projects          []ProjectRef       `json:"projects,omitempty"`
	Participants      []Participant      `json:"participants,omitempty"`
	CanonicalContext  CanonicalContext   `json:"canonicalContext"`
	ContextPromotions []ContextPromotion `json:"contextPromotions,omitempty"`
	ContextRejections []ContextRejection `json:"contextRejections,omitempty"`
	AssetRefs         []ArtifactRef      `json:"assetRefs,omitempty"`
	AssetRefEntries   []AssetRefEntry    `json:"assetRefEntries,omitempty"`
	NoteEntries       []Note             `json:"noteEntries,omitempty"`
	Notes             []string           `json:"notes,omitempty"`
	LastSeq           uint64             `json:"lastSeq"`
}
