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
	// ForOwnAgent marks a decision the agent's user made for their own agent
	// (Perimeter REQ:agent-asks-its-user). It never widens the user: the agent
	// stays capped at the user's access at every authorization.
	ForOwnAgent bool `json:"forOwnAgent,omitempty"`
	// Dormant records that the user could not read every covered object when
	// approving for their own agent; the approval takes effect only once they
	// can (Grant.EffectiveAt, event access.agent_grant.effective).
	Dormant bool `json:"dormant,omitempty"`
	// ByAgentUser marks the agent's user declining their own agent's request.
	ByAgentUser bool `json:"byAgentUser,omitempty"`
}

// Forwarding records an agent's user handing its request on to approvers. It
// is not a decision: approvers see an agent's request only once forwarded.
type Forwarding struct {
	By   Subject   `json:"by"`
	At   time.Time `json:"at"`
	Note string    `json:"note,omitempty"`
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
	// AgentUser is server-set on every agent-subject grant: the person the agent
	// asks first and whose access always caps it (the person who registered it,
	// or the person it acts for). It is absent on every other grant.
	AgentUser *Subject `json:"agentUser,omitempty"`
	// Forwarded is set once the agent's user hands the request to approvers.
	Forwarded *Forwarding `json:"forwarded,omitempty"`
	// EffectiveAt is set once, inside the grant-revision compare-and-set of the
	// agent's first allowed read after a dormant own-agent approval wakes.
	EffectiveAt *time.Time `json:"effectiveAt,omitempty"`
	// Lifetime is how long the approval lasts (Perimeter REQ:approval-lifetimes,
	// D29); absent means LifetimeDeadline.
	Lifetime Lifetime `json:"lifetime,omitempty"`
	// AgentSessionID binds a LifetimeSession grant to one server-minted agent
	// session (REQ:agent-sessions); it is set on such grants and no others.
	AgentSessionID string `json:"agentSessionId,omitempty"`
}

// Lifetime is an approval's lifetime (Perimeter REQ:approval-lifetimes, D29).
// "Always" is not a lifetime: it is a standing rule targeting an agent.
type Lifetime string

const (
	// LifetimeDeadline lasts until ExpiresAt; it is the meaning of an absent lifetime.
	LifetimeDeadline Lifetime = "deadline"
	// LifetimeSession lasts until its agent session ends or ExpiresAt, whichever
	// comes first. Agent subjects only.
	LifetimeSession Lifetime = "session"
	// LifetimeOnce is one exact write statement, consumed on commit (DAS L4, D29,
	// D30). Reserved: the MVP refuses it. Its statement data (fingerprint,
	// parameters, key set, INSERT rows, new values, preconditions) will attach as
	// an additional optional object, so the shape grows without a break.
	LifetimeOnce Lifetime = "once"
)

// AgentSession is the live agent session a control plane authorized an agent's
// call under (Perimeter REQ:agent-sessions). It is an evaluator input, set only
// on an agent's resolution: session-bound grants count only for this session
// and never beyond EndsAt (the earlier of its idle and absolute ends).
type AgentSession struct {
	ID     string    `json:"id"`
	EndsAt time.Time `json:"endsAt"`
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
	// Lifetime is the requested lifetime; the decider may narrow it, never widen it.
	Lifetime Lifetime `json:"lifetime,omitempty"`
}

type ReadPosture string

const (
	ReadAll  ReadPosture = "all"
	ReadNone ReadPosture = "none"
	// ReadExact requires a current ExactReadPolicy and a resource-bound read profile.
	ReadExact ReadPosture = "exact-v1"
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
