// Package perimeter answers read-only effective-access questions over one
// immutable snapshot of stored access facts: what a subject can read and why,
// and who can read a table and why.
//
// It is not an authorization engine. Every read decision comes from
// dataaccess.InspectTableRead, which runs the unchanged Core admission path and
// read-policy compiler and asks DALgo AssessPlan for policy metadata. The
// package performs no I/O, reads no row and never reads the clock: time is
// Snapshot.AsOf, the control-plane time the collector read the facts at.
// Answers must never be used to authorize an operation.
package perimeter

import (
	"time"

	"github.com/datatug/datatug-core/pkg/dataaccess"
)

// AnswerLease is the longest an answer stays valid after Snapshot.AsOf.
const AnswerLease = time.Minute

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

// Connection holds the stored facts of one connection.
type Connection struct {
	// Ref is the resolved, Space-bound connection identity.
	Ref dataaccess.ConnectionRef
	// Protection is the stored protection record. Nil means unreadable.
	Protection *dataaccess.ConnectionProtection
	// Exact is the sealed exact-read configuration (standing baseline and
	// mandatory denies). Required for posture exact-v1.
	Exact *dataaccess.ExactReadSnapshot
	// Grants are the stored grants on this connection in any state, with
	// ExpiresAt already clamped to the effective policy maximum.
	Grants []dataaccess.Grant
	// Inventory lists the known objects, for "what can S access" listings.
	Inventory []dataaccess.ObjectRef
	// AllowSelfApproval is the stored policy switch that lets a grant's own
	// beneficiary decide it. When false, a grant whose Decision.By equals its
	// Subject confers nothing.
	AllowSelfApproval bool
	// Incomplete names inputs of this connection that failed to load.
	Incomplete []string
}

// Resource identifies one table on one connection.
type Resource struct {
	Connection string `json:"connection"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
}

// Filter narrows a ForSubject query. Empty fields do not filter. Permissions
// defaults to data.read.
type Filter struct {
	Connections []string
	Environment string
	Permissions []dataaccess.Permission
	// IncludeDenied also lists denied resources. Denied names are not wire
	// data for an ordinary caller: set it only for a caller holding
	// access.inspect (or equivalent) over the filtered scope.
	IncludeDenied bool
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
	EnforcementNone         Enforcement = "none"
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
	Subject      *dataaccess.Subject   `json:"subject,omitempty"`
	Group        string                `json:"group,omitempty"`
	Resource     Resource              `json:"resource"`
	Permission   dataaccess.Permission `json:"permission"`
	Decision     Decision              `json:"decision"`
	Sources      []Source              `json:"sources,omitempty"`
	Restrictions []Restriction         `json:"restrictions,omitempty"`
	Blockers     []Blocker             `json:"blockers,omitempty"`
	Enforcement  Enforcement           `json:"enforcement"`
	// AllowedUntil is the admitted lease end on the AsOf clock, for allows.
	AllowedUntil *time.Time `json:"allowedUntil,omitempty"`
}

// Answer is a list of entries with its validity window.
type Answer struct {
	AsOf       time.Time `json:"asOf"`
	ValidUntil time.Time `json:"validUntil"`
	// Complete is false when any entry is indeterminate.
	Complete    bool     `json:"complete"`
	Entries     []Entry  `json:"entries"`
	Limitations []string `json:"limitations"`
}

// Reason codes owned by this package. Core denial codes and DALgo reason
// codes pass through unchanged.
const (
	CodeUnsupportedScope       = "UNSUPPORTED_SCOPE"
	CodeReadOnlyConnection     = "READ_ONLY_CONNECTION"
	CodePermissionNotModelled  = "PERMISSION_NOT_MODELLED"
	CodeUnknownPermission      = "UNKNOWN_PERMISSION"
	CodeInputIncomplete        = "INPUT_INCOMPLETE"
	CodeUnknownConnection      = "UNKNOWN_CONNECTION"
	CodeSubjectKindUnsupported = "SUBJECT_KIND_UNSUPPORTED"
	CodeMembershipUnavailable  = "MEMBERSHIP_UNAVAILABLE"
	CodeNotProjectMember       = "NOT_PROJECT_MEMBER"
	CodeProtectionUnavailable  = "PROTECTION_UNAVAILABLE"
	CodeExactPolicyUnavailable = "EXACT_POLICY_UNAVAILABLE"
	CodeEvaluationUnavailable  = "EVALUATION_UNAVAILABLE"
	CodeEvaluationInconsistent = "EVALUATION_INCONSISTENT"
	CodeSelfDecided            = "SELF_DECIDED"
)

// Limitations are stated on every answer.
var Limitations = []string{
	"Only exact-table data.read is evaluated; writes, reveal and raw queries are not grantable in this phase.",
	"Access through native database credentials outside DataTug is not visible.",
	"Agent and service subjects are not evaluated.",
}
