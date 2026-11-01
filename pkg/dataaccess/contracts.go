// Package dataaccess owns shared DataTug access contracts and a bounded,
// adapter-independent read boundary. DALgo remains the policy evaluator.
package dataaccess

import (
	"encoding/json"
	"time"
)

// Subject is assigned by the authenticated host, never by execution input.
type Subject struct {
	Realm string      `json:"realm"`
	Kind  SubjectKind `json:"kind"`
	ID    string      `json:"id"`
}

type SubjectKind string

const (
	User    SubjectKind = "user"
	Service SubjectKind = "service"
	Agent   SubjectKind = "agent"
)

// Fingerprint is resolved by the host using the engine's identity facts.
// Address is a display hint when EngineIdentity is available; without that
// identity, binding falls back to Address. Identity can name cluster lineage.
type Fingerprint struct {
	Engine         string `json:"engine"`
	Database       string `json:"database"`
	EngineIdentity string `json:"engineIdentity,omitempty"`
	Address        string `json:"address,omitempty"`
}

// ConnectionRef includes the Space because protection IDs are Space-local.
type ConnectionRef struct {
	Space       string      `json:"space"`
	ID          string      `json:"id"`
	Fingerprint Fingerprint `json:"fingerprint"`
	// LocalAuthorityID is host-owned local authority, mutually exclusive with
	// Space. Its presence never asserts enrollment or control-plane approval.
	LocalAuthorityID string `json:"localAuthorityId,omitempty"`
}

type Permission string

const (
	DataRead   Permission = "data.read"
	DataWrite  Permission = "data.write"
	DataInsert Permission = "data.insert"
	DataUpdate Permission = "data.update"
	DataDelete Permission = "data.delete"
	DataReveal Permission = "data.reveal"
	QueryTugQL Permission = "query.tugql"
	QueryRaw   Permission = "query.raw"
)

// ObjectsExact is the only optional scope feature defined for this checkpoint.
// A future selector feature must have a different name.
const ObjectsExact = "objects.exact"

// ObjectRef contains an already canonical namespace and exact object name.
// It never contains a wildcard, an identifier expression, or an address.
type ObjectRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// Selector, RecordScope, FieldScope and RequestBinding reserve the stored
// model's later-phase shape. MVP admission refuses their presence.
type Selector struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type RecordScope struct {
	Language  string          `json:"language"`
	Predicate json.RawMessage `json:"predicate"`
}

type FieldScope struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

type RequestBinding struct {
	Fingerprint string `json:"fingerprint"`
}

type GrantScope struct {
	Connection string          `json:"connection"`
	Schemas    []Selector      `json:"schemas,omitempty"`
	Objects    []ObjectRef     `json:"objects,omitempty"`
	Exclude    []Selector      `json:"exclude,omitempty"`
	Records    *RecordScope    `json:"records,omitempty"`
	Fields     *FieldScope     `json:"fields,omitempty"`
	Request    *RequestBinding `json:"request,omitempty"`
	Requires   []string        `json:"requires"`
}

type CaseRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type GrantState string

const (
	Requested GrantState = "requested"
	Approved  GrantState = "approved"
	Refused   GrantState = "refused"
	Withdrawn GrantState = "withdrawn"
	Revoked   GrantState = "revoked"
	// Expired is computed from ExpiresAt; it is never a persisted transition.
	Expired GrantState = "expired"
)

type Decision struct {
	By       Subject   `json:"by"`
	At       time.Time `json:"at"`
	Note     string    `json:"note,omitempty"`
	Self     bool      `json:"self"`
	Narrowed bool      `json:"narrowed"`
}

type Grant struct {
	ID          string       `json:"id"`
	Subject     Subject      `json:"subject"`
	RequestedBy Subject      `json:"requestedBy"`
	Permissions []Permission `json:"permissions"`
	Scope       GrantScope   `json:"scope"`
	Reason      string       `json:"reason"`
	CaseRef     *CaseRef     `json:"caseRef,omitempty"`
	NotBefore   *time.Time   `json:"notBefore,omitempty"`
	ExpiresAt   time.Time    `json:"expiresAt"`
	State       GrantState   `json:"state"`
	Decision    *Decision    `json:"decision,omitempty"`
	PriorGrant  string       `json:"priorGrant,omitempty"`
	Revision    int64        `json:"revision"`
}

// AccessRequest carries no subject, requester, approval, or identity assertion.
// The control plane derives those from authenticated host facts.
type AccessRequest struct {
	Permissions []Permission `json:"permissions"`
	Scope       GrantScope   `json:"scope"`
	Reason      string       `json:"reason"`
	ExpiresAt   time.Time    `json:"expiresAt"`
	CaseRef     *CaseRef     `json:"caseRef,omitempty"`
	PriorGrant  string       `json:"priorGrant,omitempty"`
}

type ReadPosture string

const (
	ReadAll  ReadPosture = "all"
	ReadNone ReadPosture = "none"
)

type AgentRead string

const (
	AgentInherit AgentRead = "inherit"
	AgentNone    AgentRead = "none"
)

type ConnectionProtection struct {
	ID                   string             `json:"id"`
	Fingerprint          Fingerprint        `json:"fingerprint"`
	Name                 string             `json:"name"`
	Environment          string             `json:"environment,omitempty"`
	ReadOnly             bool               `json:"readOnly"`
	ReadPosture          ReadPosture        `json:"readPosture"`
	AgentRead            AgentRead          `json:"agentRead"`
	MaxDuration          map[Permission]int `json:"maxDuration,omitempty"` // seconds
	MandatoryFineGrained bool               `json:"mandatoryFineGrained"`
	Revision             int64              `json:"revision"`
}
