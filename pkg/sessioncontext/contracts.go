// Package sessioncontext owns private, durable session context and immutable input pins.
// Source execution and Detective evaluation remain in their existing domains.
package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

var (
	ErrNotFound    = errors.New("session context: not found")
	ErrConflict    = errors.New("session context: version conflict")
	ErrInvalid     = errors.New("session context: invalid command")
	ErrForbidden   = errors.New("session context: access denied")
	ErrReadOnly    = errors.New("session context: session is read-only")
	ErrUnavailable = errors.New("session context: storage unavailable")
)

type Scope struct {
	EnvironmentID  string `json:"environmentId"`
	IdentityDomain string `json:"identityDomain"`
}
type Session struct {
	SchemaVersion   int       `json:"schemaVersion"`
	ID              string    `json:"id"`
	DatabaseID      string    `json:"databaseId"`
	ProjectID       string    `json:"projectId"`
	Owner           string    `json:"owner"`
	Title           string    `json:"title"`
	State           string    `json:"state"`
	Version         int64     `json:"version"`
	DefaultBagID    string    `json:"defaultBagId"`
	ContextSetID    string    `json:"contextSetId"`
	PendingRunCount int       `json:"pendingRunCount,omitempty"`
	BagIDs          []string  `json:"bagIds"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}
type Bag struct {
	ID           string   `json:"id"`
	SessionID    string   `json:"sessionId"`
	Title        string   `json:"title"`
	Scope        Scope    `json:"scope"`
	Version      int64    `json:"version"`
	ClearVersion int64    `json:"clearVersion,omitempty"`
	EntryIDs     []string `json:"entryIds"`
}
type Subject struct {
	ConceptID       string   `json:"conceptId"`
	FieldID         string   `json:"fieldId"`
	FieldMappingRef string   `json:"fieldMappingRef"`
	GraphResource   string   `json:"graphResource,omitempty"`
	GraphRevision   string   `json:"graphRevision,omitempty"`
	ModelRef        string   `json:"modelRef,omitempty"`
	ModelRevision   string   `json:"modelRevision,omitempty"`
	Record          string   `json:"record,omitempty"`
	Fields          []string `json:"fields,omitempty"`
}
type KeyComponent struct {
	Field string                 `json:"field"`
	Value apicontract.TypedValue `json:"value"`
}
type EntityKey struct {
	Components []KeyComponent `json:"components"`
}

// ValueEnvelope keeps integers/decimals lossless and composite keys structurally typed.
// Sets may be empty; an empty set is never interpreted as an unfiltered source.
type ValueEnvelope struct {
	SchemaVersion int                      `json:"schemaVersion"`
	Kind          string                   `json:"kind"`
	Scalar        *apicontract.TypedValue  `json:"scalar,omitempty"`
	Values        []apicontract.TypedValue `json:"values,omitempty"`
	KeyFields     []string                 `json:"keyFields,omitempty"`
	Keys          []EntityKey              `json:"keys,omitempty"`
}
type Entry struct {
	ID                string  `json:"id"`
	SessionID         string  `json:"sessionId"`
	BagID             string  `json:"bagId"`
	Alias             string  `json:"alias,omitempty"`
	Subject           Subject `json:"subject"`
	CurrentRevisionID string  `json:"currentRevisionId"`
	Version           int64   `json:"version"`
}
type EntryRevision struct {
	ID               string        `json:"id"`
	EntryID          string        `json:"entryId"`
	BagID            string        `json:"bagId"`
	ParentRevisionID string        `json:"parentRevisionId,omitempty"`
	Subject          Subject       `json:"subject"`
	Value            ValueEnvelope `json:"value"`
	CreatedAt        time.Time     `json:"createdAt"`
}
type EntryRef struct {
	EntryID    string `json:"entryId"`
	RevisionID string `json:"revisionId"`
}

// A slot with an empty bagId is a cleared tombstone. Its version prevents ABA reuse.
type AppliedSlot struct {
	Scope      Scope      `json:"scope"`
	BagID      string     `json:"bagId"`
	BagVersion int64      `json:"bagVersion"`
	Version    int64      `json:"version"`
	Entries    []EntryRef `json:"entries"`
}
type AppliedContextSet struct {
	ID        string        `json:"id"`
	SessionID string        `json:"sessionId"`
	Enabled   bool          `json:"enabled"`
	Slots     []AppliedSlot `json:"slots"`
	CreatedAt time.Time     `json:"createdAt"`
}
type Snapshot struct {
	Session    Session           `json:"session"`
	Bags       []Bag             `json:"bags"`
	Entries    []Entry           `json:"entries"`
	Revisions  []EntryRevision   `json:"revisions"`
	ContextSet AppliedContextSet `json:"contextSet"`
}
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

// Principal is supplied by the authorized server boundary, never decoded from a command.
type Principal struct {
	// Executor is set only by the server runtime when persisting an actual outcome.
	Executor   bool
	Actor      string
	DatabaseID string
	ProjectID  string
}
type Access struct {
	Principal Principal
	Action    string
	SessionID string
	Scope     *Scope
}
type Authorizer func(context.Context, Access) error

// Receipt and audit contain references and counts only, never source/input values or titles.
type Receipt struct {
	Scope           *Scope    `json:"scope,omitempty"`
	OpID            string    `json:"opId"`
	Actor           string    `json:"actor"`
	Action          string    `json:"action"`
	PayloadDigest   string    `json:"payloadDigest"`
	SessionID       string    `json:"sessionId"`
	SessionVersion  int64     `json:"sessionVersion"`
	ResourceID      string    `json:"resourceId,omitempty"`
	ResourceVersion int64     `json:"resourceVersion,omitempty"`
	At              time.Time `json:"at"`
}
type AuditEvent struct {
	OpID       string    `json:"opId"`
	Actor      string    `json:"actor"`
	Action     string    `json:"action"`
	SessionID  string    `json:"sessionId"`
	ResourceID string    `json:"resourceId,omitempty"`
	Count      int       `json:"count"`
	At         time.Time `json:"at"`
}

// Command is shared by HTTP/terminal adapters. IDs are opaque, path-safe and stable.
// expectedVersion is the session version. expectedEntryVersion=0 creates an entry.
type Command struct {
	OpID                 string         `json:"opId"`
	Action               string         `json:"action"`
	SessionID            string         `json:"sessionId"`
	ExpectedVersion      int64          `json:"expectedVersion,omitempty"`
	DatabaseID           string         `json:"databaseId,omitempty"`
	ProjectID            string         `json:"projectId,omitempty"`
	BagID                string         `json:"bagId,omitempty"`
	ExpectedBagVersion   int64          `json:"expectedBagVersion,omitempty"`
	ExpectedSlotVersion  int64          `json:"expectedSlotVersion,omitempty"`
	ExpectedContextSetID string         `json:"expectedContextSetId,omitempty"`
	EntryID              string         `json:"entryId,omitempty"`
	ExpectedEntryVersion int64          `json:"expectedEntryVersion,omitempty"`
	RevisionID           string         `json:"revisionId,omitempty"`
	Title                string         `json:"title,omitempty"`
	Alias                string         `json:"alias,omitempty"`
	Scope                *Scope         `json:"scope,omitempty"`
	Subject              *Subject       `json:"subject,omitempty"`
	Value                *ValueEnvelope `json:"value,omitempty"`
	Selection            []EntryRef     `json:"selection,omitempty"`
	Enabled              *bool          `json:"enabled,omitempty"`
	Transition           string         `json:"transition,omitempty"`
	// M3 input pins and durable run routing. Result rows remain in the existing result store.
	InputSnapshotID    string                            `json:"inputSnapshotId,omitempty"`
	RunID              string                            `json:"runId,omitempty"`
	Kind               string                            `json:"kind,omitempty"`
	DefinitionID       string                            `json:"definitionId,omitempty"`
	DefinitionRevision string                            `json:"definitionRevision,omitempty"`
	DefinitionDocument json.RawMessage                   `json:"definitionDocument,omitempty"`
	ExecutionDocument  json.RawMessage                   `json:"executionDocument,omitempty"`
	Attachments        []EntryRef                        `json:"attachments,omitempty"`
	Text               string                            `json:"text,omitempty"`
	ExpectedRunVersion int64                             `json:"expectedRunVersion,omitempty"`
	RunState           string                            `json:"runState,omitempty"`
	Widgets            []WidgetOutcome                   `json:"widgets,omitempty"`
	ResultRefs         []string                          `json:"resultRefs,omitempty"`
	ErrorCode          string                            `json:"errorCode,omitempty"`
	CaseID             string                            `json:"caseId,omitempty"`
	DetectiveRunID     string                            `json:"detectiveRunId,omitempty"`
	ResolvedParameters map[string]apicontract.TypedValue `json:"resolvedParameters,omitempty"`
}
type CommandResult struct {
	Receipt Receipt `json:"receipt"`
}
type InputSnapshot struct {
	ID                 string                            `json:"id"`
	SessionID          string                            `json:"sessionId"`
	ContextSetID       string                            `json:"contextSetId"`
	Scope              Scope                             `json:"scope"`
	Entries            []EntryRef                        `json:"entries"`
	DefinitionDigest   string                            `json:"definitionDigest"`
	DefinitionDocument json.RawMessage                   `json:"definitionDocument"`
	ResolvedParameters map[string]apicontract.TypedValue `json:"resolvedParameters,omitempty"`
	ParametersOrigin   string                            `json:"parametersOrigin,omitempty"`
	CreatedAt          time.Time                         `json:"createdAt"`
}
type WidgetOutcome struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	ResultRef string `json:"resultRef,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}
type Run struct {
	ID                 string                            `json:"id"`
	SessionID          string                            `json:"sessionId"`
	InputSnapshotID    string                            `json:"inputSnapshotId"`
	Kind               string                            `json:"kind"`
	DefinitionID       string                            `json:"definitionId"`
	DefinitionRevision string                            `json:"definitionRevision"`
	State              string                            `json:"state"`
	OutcomeOrigin      string                            `json:"outcomeOrigin,omitempty"`
	ExecutionAuthority string                            `json:"executionAuthority"`
	ExecutionDigest    string                            `json:"executionDigest,omitempty"`
	ExecutionDocument  json.RawMessage                   `json:"executionDocument,omitempty"`
	ResolvedParameters map[string]apicontract.TypedValue `json:"resolvedParameters,omitempty"`
	Version            int64                             `json:"version"`
	Widgets            []WidgetOutcome                   `json:"widgets,omitempty"`
	ResultRefs         []string                          `json:"resultRefs,omitempty"`
	ErrorCode          string                            `json:"errorCode,omitempty"`
	CreatedAt          time.Time                         `json:"createdAt"`
	CompletedAt        *time.Time                        `json:"completedAt,omitempty"`
}
type Message struct {
	ID              string    `json:"id"`
	SessionID       string    `json:"sessionId"`
	InputSnapshotID string    `json:"inputSnapshotId"`
	Role            string    `json:"role"`
	Text            string    `json:"text"`
	CreatedAt       time.Time `json:"createdAt"`
}

// EvidenceHandoff cannot replace Detective's native CaseRunLink transaction.
// A worker finalizes the native Detective operation, then marks this handoff delivered.
type EvidenceHandoff struct {
	ID              string `json:"id"`
	SessionID       string `json:"sessionId"`
	RunID           string `json:"runId"`
	InputSnapshotID string `json:"inputSnapshotId"`
	CaseID          string `json:"caseId"`
	DetectiveRunID  string `json:"detectiveRunId"`
	State           string `json:"state"`
	ErrorCode       string `json:"errorCode,omitempty"`
}

// Repository implementations commit heads, immutable records, receipt and audit in one
// authority. List pages are bounded and owner-filtered before records reach clients.
type Repository interface {
	View(context.Context, func(Reader) error) error
	Transact(context.Context, func(Transaction) error) error
}
type Reader interface {
	Load(context.Context, string, string, any) (int64, error)
	List(context.Context, string, string, string, int) ([]json.RawMessage, string, error)
}
type Transaction interface {
	Reader
	Save(context.Context, string, string, any, int64, string, string) error
}
