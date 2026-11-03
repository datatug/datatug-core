// Package perimeter answers read-only effective-access questions over one
// immutable snapshot of stored access facts: what a subject can read and why,
// and who can read a table and why.
//
// It is not an authorization engine. Every read decision comes from
// dataaccess.InspectTableRead, which runs the unchanged Core admission path and
// read-policy compiler and asks DALgo AssessPlan for policy metadata, on exactly
// the resolution the backend resolver would build. The package's own rules
// (membership, self-decided grants, unreadable inputs) can only turn an allow
// into a deny or an unknown, never the reverse. It performs no I/O, reads no
// row and never reads the clock: time is Snapshot.AsOf, the control-plane time
// the collector read the facts at. Answers must never be used to authorize an
// operation.
package perimeter

import (
	"time"

	"github.com/datatug/datatug-core/pkg/dataaccess"
)

// AnswerLease is the longest an answer stays valid after Snapshot.AsOf.
const AnswerLease = time.Minute

// MaxGrantsPerSubject mirrors the backend's own-grant page cap: a subject with
// more stored grants on one connection cannot be resolved.
const MaxGrantsPerSubject = 100

// Snapshot is one immutable, internally consistent read of the stored access
// facts of one project, taken by a trusted collector at control-plane time AsOf.
type Snapshot struct {
	// AsOf is control-plane time. Expiry is measured against it, never the
	// local clock.
	AsOf time.Time
	// Connections are the project's protected connections.
	Connections []Connection
	// Members are the verified human project members. Nil means membership
	// could not be read; every subject answer is then indeterminate.
	Members []dataaccess.Subject
	// Incomplete names inputs that failed to load. Any entry makes every
	// read answer indeterminate.
	Incomplete []string
}

// StoredGrant is one stored grant with the connection fingerprint it was
// recorded against.
type StoredGrant struct {
	dataaccess.Grant
	Fingerprint dataaccess.Fingerprint
}

// Connection holds the stored facts of one connection.
type Connection struct {
	// Ref is the resolved, Space-bound connection identity.
	Ref dataaccess.ConnectionRef
	// Protection is the stored protection record. Nil means unreadable.
	Protection *dataaccess.ConnectionProtection
	// Exact is the sealed exact-read configuration (standing baseline and
	// mandatory denies). Required for posture exact-v1.
	Exact *dataaccess.ExactReadSnapshot
	// Grants are every stored grant on this connection in any state, with
	// ExpiresAt already clamped to the effective policy maximum. A subject
	// with any grant whose fingerprint differs from Ref's is unresolvable, as
	// in the backend resolver, and gets indeterminate answers.
	Grants []StoredGrant
	// Inventory lists the known objects, for "what can S access" listings.
	Inventory []dataaccess.ObjectRef
	// AllowSelfApproval is the stored policy switch that lets a grant's own
	// beneficiary decide it. When false, a grant whose Decision.By equals its
	// Subject confers nothing.
	AllowSelfApproval bool
	// Incomplete names inputs of this connection that failed to load.
	Incomplete []string
}

// Resource identifies one table on one connection. A connection-level row
// has an empty Namespace and Name.
type Resource struct {
	Connection string `json:"connection"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
}

// Filter narrows a ForSubject query. Empty fields do not filter. Permissions
// defaults to data.read.
type Filter struct {
	Connections []string
	Environment string
	Permissions []dataaccess.Permission
	// IncludeDenied lists every inventory object with full blocker detail.
	// Without it, a self-view lists only allowed objects and objects named in
	// the subject's own grants, with deny detail reduced to READ_DENIED, and
	// folds every other unknown into one row per connection with no table
	// name. Denied and hidden names are not wire data for an ordinary caller:
	// set it only for a caller holding access.inspect over the scope.
	IncludeDenied bool
}

// ExplainOptions controls Explain's disclosure.
type ExplainOptions struct {
	// Detail returns the policy-level blockers and candidate sources of a
	// deny. They disclose mandatory policy and whether a name exists: set it
	// only for a caller holding access.inspect over the resource. Without it a
	// deny carries only READ_DENIED.
	Detail bool
}

// Decision is the outcome of one (subject, permission, resource) question.
type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
	// Indeterminate means an input was missing, unreadable or unsupported.
	// It is shown as "unknown" and never as allowed.
	Indeterminate Decision = "indeterminate"
)

// Enforcement states how the answer is enforced.
type Enforcement string

const (
	EnforcementServer       Enforcement = "server"
	EnforcementHonestClient Enforcement = "honest-client"
)

// Source kinds that can contribute an allow.
const (
	SourcePosture  = "posture"
	SourceBaseline = "baseline"
	SourceGrant    = "grant"
)

// Ref names the record a blocker or restriction comes from.
type Ref struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Source is one contributing allow. Order within an entry is deterministic:
// posture, then baseline, then grants by expiry and ID.
type Source struct {
	Kind      string     `json:"kind"`
	ID        string     `json:"id"`
	Target    string     `json:"target,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// Restriction is a mandatory narrowing that applies to an allowed answer.
type Restriction struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Effect string `json:"effect"`
}

// Blocker is a fixed reason code, never free text.
type Blocker struct {
	Code   string `json:"code"`
	Phase  string `json:"phase,omitempty"`
	Source *Ref   `json:"source,omitempty"`
}

// GroupProjectMembers is the Group of the single "every project member" row.
const GroupProjectMembers = "project-members"

// Entry is one answer row: the permission model's answer shape (§7.3).
type Entry struct {
	// Subject is nil for a group row.
	Subject    *dataaccess.Subject   `json:"subject,omitempty"`
	Group      string                `json:"group,omitempty"`
	Resource   Resource              `json:"resource"`
	Permission dataaccess.Permission `json:"permission,omitempty"`
	Decision   Decision              `json:"decision"`
	// Sources are the allows that admitted the read. Only allows carry them.
	Sources      []Source      `json:"sources,omitempty"`
	Restrictions []Restriction `json:"restrictions,omitempty"`
	Blockers     []Blocker     `json:"blockers,omitempty"`
	// Candidates are, on a detailed deny, the inputs that would have allowed
	// the read but did not admit it.
	Candidates []Source `json:"candidates,omitempty"`
	// Enforcement is empty when the connection is unknown.
	Enforcement Enforcement `json:"enforcement,omitempty"`
	// AllowedUntil is the admitted lease end on the AsOf clock, for allows.
	AllowedUntil *time.Time `json:"allowedUntil,omitempty"`
}

// Answer is a list of entries with its validity window.
type Answer struct {
	AsOf       time.Time `json:"asOf"`
	ValidUntil time.Time `json:"validUntil"`
	// Complete is false when any input was unreadable or any entry is
	// indeterminate.
	Complete bool    `json:"complete"`
	Entries  []Entry `json:"entries"`
	// HiddenSubjects is left zero by this package; a host that removes rows
	// the asker may not see under contact visibility counts them here.
	HiddenSubjects int      `json:"hiddenSubjects"`
	Limitations    []string `json:"limitations"`
}

// Reason codes owned by this package. Core denial and DALgo reason codes pass
// through unchanged on detailed answers.
const (
	CodeReadDenied             = "READ_DENIED"
	CodeUnsupportedScope       = "UNSUPPORTED_SCOPE"
	CodeReadOnlyConnection     = "READ_ONLY_CONNECTION"
	CodePermissionNotModelled  = "PERMISSION_NOT_MODELLED"
	CodeUnknownPermission      = "UNKNOWN_PERMISSION"
	CodeInputIncomplete        = "INPUT_INCOMPLETE"
	CodeUnknownConnection      = "UNKNOWN_CONNECTION"
	CodeLocalConnection        = "LOCAL_CONNECTION_UNSUPPORTED"
	CodeSubjectKindUnsupported = "SUBJECT_KIND_UNSUPPORTED"
	CodeMembershipUnavailable  = "MEMBERSHIP_UNAVAILABLE"
	CodeNotProjectMember       = "NOT_PROJECT_MEMBER"
	CodeProtectionUnavailable  = "PROTECTION_UNAVAILABLE"
	CodeGrantBindingMismatch   = "GRANT_BINDING_MISMATCH"
	CodeGrantListTruncated     = "GRANT_LIST_TRUNCATED"
	CodeExactPolicyUnavailable = "EXACT_POLICY_UNAVAILABLE"
	CodeEvaluationUnavailable  = "EVALUATION_UNAVAILABLE"
	CodeEvaluationInconsistent = "EVALUATION_INCONSISTENT"
	CodeSelfDecided            = "SELF_DECIDED"
	CodeResourcesNotEnumerated = "RESOURCES_NOT_ENUMERATED"
)

// Limitations are stated on every answer.
var Limitations = []string{
	"Only exact-table data.read is evaluated; writes, reveal and raw queries are not grantable in this phase.",
	"Access through native database credentials outside DataTug is not visible.",
	"Agent and service subjects are not evaluated.",
	"Local-policy (honest-client) connections are not evaluated.",
}
