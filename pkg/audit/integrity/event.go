package integrity

import "time"

// Event is one audit event in the shape of permission model §19.2. Every
// person-valued field holds an [ActorRef], never a real user id.
//
// ExpireAt (the Firestore TTL field) is deliberately absent: it is set at
// sealing and is not committed. Optional numbers are pointers so that zero and
// absent stay distinct.
type Event struct {
	ID      string    `json:"id"`
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`              // access | activity
	Action  string    `json:"action"`            // access.approved | operation.authorized | ...
	Tier    string    `json:"tier"`              // server-enforced | client-reported | advisory
	Outcome string    `json:"outcome,omitempty"` // allow | deny | error (activity only)
	Reason  string    `json:"reason,omitempty"`  // fixed code on deny/error, or free text on access events

	Actor     Actor     `json:"actor"`
	Access    Parties   `json:"access"`
	Resource  Resource  `json:"resource"`
	Operation Operation `json:"operation"`
	Authority Authority `json:"authority"`
	CaseRef   *CaseRef  `json:"caseRef,omitempty"`
	Execution *ExecRef  `json:"executionRef,omitempty"`
	Statement *StmtHMAC `json:"statementDigest,omitempty"`
	Window    string    `json:"window"`               // WindowID.Key of the arrival window
	Occurred  time.Time `json:"occurredAt,omitempty"` // original time of a late event
}

// Actor is who acted.
type Actor struct {
	Ref              ActorRef `json:"ref"`
	Kind             string   `json:"kind"` // user | agent | service
	CredentialFamily string   `json:"credentialFamily"`
	ClientID         string   `json:"clientId"`
	OnBehalfOf       ActorRef `json:"onBehalfOf,omitempty"`
}

// Parties are the people named by an access-control event (§19.4 erasure).
type Parties struct {
	Requester   ActorRef `json:"requester,omitempty"`
	Approver    ActorRef `json:"approver,omitempty"`
	Beneficiary ActorRef `json:"beneficiary,omitempty"`
}

// Resource is what the event touched: references and names, never values.
type Resource struct {
	Space       string      `json:"space"`
	Project     string      `json:"project"`
	Environment string      `json:"environment,omitempty"`
	Connection  string      `json:"connection,omitempty"`
	Fingerprint string      `json:"fingerprint,omitempty"`
	Objects     []ObjectRef `json:"objects,omitempty"`
	Fields      []string    `json:"fields,omitempty"`
	Bag         *BagRef     `json:"bag,omitempty"`
}

// ObjectRef names one object (table, view, collection).
type ObjectRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// BagRef identifies the Context Bag revision a scope came from.
type BagRef struct {
	Session  string `json:"session"`
	Bag      string `json:"bag"`
	Revision int64  `json:"revision"`
}

// Operation describes an activity event.
type Operation struct {
	Op            string `json:"op,omitempty"`
	CorrelationID string `json:"correlationId,omitempty"`
	RecordCount   *int64 `json:"recordCount,omitempty"`
	Pages         *int64 `json:"pages,omitempty"`
	// Denials is the coalesced count of an operation.denied event.
	Denials *int64 `json:"denials,omitempty"`
}

// Authority is the resolution the boundary used.
type Authority struct {
	Grants             []string `json:"grants,omitempty"`
	Decisions          []string `json:"decisions,omitempty"`
	Standing           []string `json:"standing,omitempty"`
	Roles              []string `json:"roles,omitempty"`
	TeamBasis          []string `json:"teamBasis,omitempty"`
	PolicyFingerprint  string   `json:"policyFingerprint,omitempty"`
	ProtectionRevision *int64   `json:"protectionRevision,omitempty"`
}

// CaseRef links an incident or other case.
type CaseRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// ExecRef links an execution record.
type ExecRef struct {
	StoreID string `json:"storeId"`
	Project string `json:"project"`
	ID      string `json:"id"`
}

// StmtHMAC is a keyed statement digest.
type StmtHMAC struct {
	KeyVersion int64  `json:"keyVersion"`
	HMAC       string `json:"hmac"`
}
