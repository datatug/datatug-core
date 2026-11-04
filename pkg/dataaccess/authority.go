package dataaccess

import (
	"context"
	"fmt"
	"time"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
)

type ResolveRequest struct {
	Subject    Subject       `json:"subject"`
	Connection ConnectionRef `json:"connection"`
}

// Resolution comes only from the injected authenticated authority port. The
// port verifies current membership and protection state, and refuses failure.
// ServerTime and ValidUntil are verified server facts, not local clock values.
// ServerTime must be fresh for this resolve call. A cached response must not be
// replayed as new authority; a future caching adapter must preserve its original
// age and deadline rather than rebase either onto the next call's local time.
type Resolution struct {
	Subject    Subject              `json:"subject"`
	Connection ConnectionRef        `json:"connection"`
	Protection ConnectionProtection `json:"protection"`
	Grants     []Grant              `json:"grants"`
	ServerTime time.Time            `json:"serverTime"`
	ValidUntil time.Time            `json:"validUntil"`
	ExactRead  *ExactReadPolicy     `json:"exactRead,omitempty"`
	// AgentUser is required for an agent subject and absent otherwise: the
	// fresh resolution of the person the agent belongs to (the person who
	// registered it, or the person it acts for). Every authorization intersects
	// the agent's own perimeter with it (Perimeter REQ:agent-access), so the
	// user's grants, posture and memberships are inputs of every agent answer.
	AgentUser *Resolution `json:"agentUser,omitempty"`
	// AgentSession is the live agent session of this call, when there is one.
	// Without it an agent keeps standing access and deadline approvals, and no
	// session-bound grant may be present.
	AgentSession *AgentSession `json:"agentSession,omitempty"`
}

type Resolver func(context.Context, ResolveRequest) (Resolution, error)

// Authority binds one host-assigned subject and one resolved connection. It
// does not accept a caller authorization token or a local-owner boolean.
type Authority struct {
	request ResolveRequest
	resolve Resolver
	now     func() time.Time
}

func NewAuthority(subject Subject, connection ConnectionRef, resolve Resolver) (*Authority, error) {
	if err := subject.Validate(); err != nil {
		return nil, err
	}
	fingerprint := connection.Fingerprint
	if resolve == nil || connection.LocalAuthorityID != "" || !canonical(connection.Space) || !canonical(connection.ID) ||
		!canonical(fingerprint.Engine) || !canonical(fingerprint.Database) ||
		(fingerprint.EngineIdentity == "" && fingerprint.Address == "") {
		return nil, fmt.Errorf("dataaccess: resolver and resolved connection are required")
	}
	return &Authority{request: ResolveRequest{Subject: subject, Connection: connection}, resolve: resolve, now: time.Now}, nil
}

// Database deliberately exposes no Schema, Adapter, native/optional capability,
// underlying handle, or read-write transaction opener. Those surfaces require
// separate integration proof. Typed writes remain callable only to return a
// DALgo policy denial before dispatch. Recordset queries are refused; the query
// profile supports only frozen, unconditional single-source key enumeration,
// with an explicit schema required for exact-object grants. Object-only retained
// result transport and transactions remain unavailable at this checkpoint.
// Point reads stage maps, non-nil pointers and fixed-length slices; nil, scalar
// or inaccessible targets are refused before dispatch.
// This additive boundary is a Task 4 checkpoint, not whole-task integration.
type Database interface {
	dal.ReadwriteSession
	dal.ReadTransactionCoordinator
}

// SecureDB is a trusted composition point: the host supplies the database that
// corresponds to the resolved ConnectionRef. Only the secured interface leaves
// it. Production opener wiring and its architecture ratchet are later work.
func (a *Authority) SecureDB(db dal.DB) (Database, error) {
	return a.secureDB(db, nil)
}

func (a *Authority) secureDB(db dal.DB, verifier TableIdentityVerifier) (Database, error) {
	if a == nil || a.resolve == nil {
		return nil, fmt.Errorf("dataaccess: authority is required")
	}
	secured, err := access.SecureDB(db, access.RequireContextPolicy(), access.WithDatabasePolicyProvider(policyForOperation))
	if err != nil {
		return nil, err
	}
	writer, _ := secured.(dal.WriteSession)
	// No write rule exists, so even an adapter without this optional surface
	// gets the same policy denial without dereferencing its absent writer.
	if writer == nil {
		writer = access.SecureWriteSession(nil, readPolicy)
	}
	session := guardedSession{authority: a, read: secured, write: writer, identityDB: db, identityVerifier: verifier}
	return &database{guardedSession: session, secured: secured}, nil
}

type lease struct {
	deadline     time.Time
	started      time.Time
	serverTime   time.Time
	digest       string
	policy       access.Policy
	request      *access.Request                            // immutable DALgo-mapped resources of a frozen operation
	identity     interface{ verify(context.Context) error } // immutable native binding for this operation
	exactProfile bool                                       // only Core table/structured entrypoints set this marker
}

type authorizationKey struct{}

var readPolicy = access.MustPolicy("datatug-core-read-only", access.Root(access.Allow(access.Read, "read-only")))

func policyForOperation(ctx context.Context) ([]access.Policy, error) {
	authorization, _ := ctx.Value(authorizationKey{}).(*lease)
	if authorization == nil {
		return nil, fmt.Errorf("dataaccess: operation authorization is required")
	}
	return []access.Policy{authorization.policy}, nil
}

func sameFingerprint(a, b Fingerprint) bool {
	if a.Engine != b.Engine || a.Database != b.Database || a.EngineIdentity != b.EngineIdentity {
		return false
	}
	return a.EngineIdentity != "" || (a.Address != "" && a.Address == b.Address)
}

func (a *Authority) denial(capability Permission, code string) error {
	return &Denial{Connection: a.request.Connection.ID, Capability: capability, Code: code}
}

// authorize deliberately resolves on each operation/page at this checkpoint.
// It retains no caller-mutable grant, permission, policy or subject slices.
func (a *Authority) authorize(ctx context.Context) (*lease, error) {
	return a.authorizeRequest(ctx, nil)
}

func (a *Authority) authorizeRequest(ctx context.Context, request *access.Request) (*lease, error) {
	started := a.now()
	resolution, err := a.resolve(sourceReadContext{ctx}, a.request)
	if err != nil {
		return nil, a.denial(DataRead, "AUTHORITY_UNAVAILABLE")
	}
	if resolution.Subject != a.request.Subject || !sameConnection(resolution.Connection, a.request.Connection) ||
		resolution.Protection.ID != a.request.Connection.ID ||
		!sameFingerprint(resolution.Protection.Fingerprint, a.request.Connection.Fingerprint) {
		return nil, a.denial(DataRead, "BINDING_MISMATCH")
	}
	if a.request.Connection.LocalAuthorityID != "" && (len(resolution.Grants) != 0 || resolution.AgentUser != nil && len(resolution.AgentUser.Grants) != 0) {
		return nil, a.denial(DataRead, "LOCAL_GRANTS_UNSUPPORTED")
	}
	resolution, err = canonicalExactResolution(resolution)
	if err != nil {
		return nil, a.denial(DataRead, "AUTHORITY_UNSUPPORTED")
	}
	exactProfile, _ := ctx.Value(exactReadProfileKey{}).(bool)
	if resolution.Protection.ReadPosture == ReadExact && (!exactProfile || request == nil || !resolution.Protection.ReadOnly) {
		return nil, a.denial(DataRead, "EXACT_PROFILE_REQUIRED")
	}
	if resolution.ServerTime.IsZero() || !resolution.ValidUntil.After(resolution.ServerTime) ||
		(resolution.Protection.ReadPosture != ReadAll && resolution.Protection.ReadPosture != ReadNone && resolution.Protection.ReadPosture != ReadExact) ||
		(resolution.Protection.AgentRead != AgentInherit && resolution.Protection.AgentRead != AgentNone) ||
		resolution.Protection.MandatoryFineGrained {
		return nil, a.denial(DataRead, "AUTHORITY_UNSUPPORTED")
	}
	if err := a.checkGrantBinding(resolution); err != nil {
		return nil, err
	}
	if a.request.Subject.Kind == Agent && resolution.Protection.AgentRead == AgentNone {
		return nil, a.denial(DataRead, "READ_DENIED")
	}
	policy := newOperationReadPolicy(resolution, started, a.now)
	deadline, err := policy.deadlineFor(request)
	if err != nil {
		return nil, a.denial(DataRead, "READ_DENIED")
	}
	if a.request.Subject.Kind == Agent {
		// The agent never exceeds its user: intersect, never union. A read the
		// agent's own perimeter admits but its user's does not is refused with
		// the user's missing access named (D27).
		policy = policy.intersect(newOperationReadPolicy(*resolution.AgentUser, started, a.now))
		if deadline, err = policy.deadlineFor(request); err != nil {
			return nil, a.denial(DataRead, "AGENT_USER_LACKS_ACCESS")
		}
	}
	// Starting the interval before the network call subtracts its whole latency.
	// The monotonic local interval does not trust the machine's wall-clock skew.
	digest, err := resolutionDigest(resolution)
	if err != nil {
		return nil, a.denial(DataRead, "AUTHORITY_UNSUPPORTED")
	}
	return &lease{deadline: deadline, started: started, serverTime: resolution.ServerTime, digest: digest, policy: policy, request: copyReadRequest(request), exactProfile: exactProfile}, nil
}

// checkGrantBinding admits only grants whose subject is the resolved subject.
// An agent may hold its own approvals (D23), bound to the same agent user as
// its resolution; a human grant never reaches an agent, and an agent grant
// never reaches a human. The agent user's resolution is bound the same way.
func (a *Authority) checkGrantBinding(resolution Resolution) error {
	var user *Subject
	if a.request.Subject.Kind == Agent {
		u := resolution.AgentUser
		if u == nil || u.AgentUser != nil || u.Subject.Kind != User || u.Subject.Validate() != nil || u.Subject.Realm != a.request.Subject.Realm ||
			!sameConnection(u.Connection, a.request.Connection) || !sameProtection(u.Protection, resolution.Protection) ||
			u.ServerTime.IsZero() || !u.ValidUntil.After(u.ServerTime) ||
			// The user's resolution is as fresh as the agent's: same server
			// instant and no longer validity, never a cached older answer.
			!u.ServerTime.Equal(resolution.ServerTime) || u.ValidUntil.After(resolution.ValidUntil) ||
			// The session belongs to the agent's call, never to its user's.
			u.AgentSession != nil || (resolution.AgentSession != nil && (!canonical(resolution.AgentSession.ID) || resolution.AgentSession.EndsAt.IsZero())) {
			return a.denial(DataRead, "BINDING_MISMATCH")
		}
		user = &u.Subject
		for _, grant := range u.Grants {
			if grant.ValidateMVP() != nil {
				return unsupported(a.request.Connection.ID)
			}
			if grant.Subject != u.Subject || grant.Scope.Connection != a.request.Connection.ID {
				return a.denial(DataRead, "BINDING_MISMATCH")
			}
		}
	} else if resolution.AgentUser != nil || resolution.AgentSession != nil {
		return a.denial(DataRead, "BINDING_MISMATCH")
	}
	for _, grant := range resolution.Grants {
		if err := grant.ValidateMVP(); err != nil {
			return unsupported(a.request.Connection.ID)
		}
		if grant.Subject != a.request.Subject || grant.Scope.Connection != a.request.Connection.ID ||
			(user == nil) != (grant.AgentUser == nil) || (user != nil && *grant.AgentUser != *user) ||
			// A session-bound grant counts only in its own live session (D29).
			(grant.Lifetime == LifetimeSession && !sessionBound(resolution, grant)) {
			return a.denial(DataRead, "BINDING_MISMATCH")
		}
	}
	return nil
}

// sessionBound reports whether a session-bound grant belongs to the
// resolution's live agent session.
func sessionBound(resolution Resolution, grant Grant) bool {
	return resolution.AgentSession != nil && resolution.AgentSession.ID == grant.AgentSessionID
}

func (a *Authority) check(ctx context.Context, authorization *lease) error {
	if !a.now().Before(authorization.deadline) {
		return a.denial(DataRead, "AUTHORIZATION_EXPIRED")
	}
	return ctx.Err()
}

func (a *Authority) prepare(ctx context.Context, bound *lease) (context.Context, *lease, context.CancelFunc, error) {
	return a.prepareRequest(ctx, bound, nil)
}

func (a *Authority) prepareRequest(ctx context.Context, bound *lease, request *access.Request) (context.Context, *lease, context.CancelFunc, error) {
	return a.prepareWithCeiling(ctx, bound, request, nil)
}

// onCeiling is an internal reader-lifetime hook, invoked before fact I/O and
// without an ownership lock. It may only shorten that reader's local lifetime.
func (a *Authority) prepareWithCeiling(ctx context.Context, bound *lease, request *access.Request, onCeiling func(*lease)) (context.Context, *lease, context.CancelFunc, error) {
	if bound != nil {
		if err := a.check(ctx, bound); err != nil {
			return nil, nil, nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	if request == nil && bound != nil {
		request = bound.request
	}
	if bound != nil && bound.exactProfile {
		ctx = withExactReadProfile(ctx)
	}
	authorization, err := a.authorizeRequest(ctx, request)
	if err != nil {
		return nil, nil, nil, err
	}
	if bound != nil && bound.request != nil && bound.digest != authorization.digest {
		return nil, nil, nil, a.denial(DataRead, "AUTHORIZATION_CHANGED")
	}
	if bound != nil && bound.deadline.Before(authorization.deadline) {
		authorization.deadline = bound.deadline
	}
	if err := a.check(ctx, authorization); err != nil {
		return nil, nil, nil, err
	}
	if bound != nil {
		authorization.identity = bound.identity
	}
	// Admission is internal to the captured lease; never pass its marker to
	// resolver/source/native callbacks or returned operation contexts.
	ctx, cancel := context.WithDeadline(sourceReadContext{ctx}, authorization.deadline)
	if onCeiling != nil {
		onCeiling(authorization)
		if err := a.check(ctx, authorization); err != nil {
			cancel()
			return nil, nil, nil, err
		}
	}
	if authorization.identity != nil {
		if err := authorization.identity.verify(sourceReadContext{ctx}); err != nil {
			cancel()
			return nil, nil, nil, a.denial(DataRead, "TABLE_IDENTITY_UNVERIFIED")
		}
		if err := a.check(ctx, authorization); err != nil {
			cancel()
			return nil, nil, nil, err
		}
	}
	ctx = context.WithValue(ctx, authorizationKey{}, authorization)
	ctx = access.WithPolicy(ctx, authorization.policy)
	return ctx, authorization, cancel, nil
}

func (a *Authority) recheck(ctx context.Context, bound *lease) error {
	return a.recheckWithCeiling(ctx, bound, nil)
}

func (a *Authority) recheckWithCeiling(ctx context.Context, bound *lease, onCeiling func(*lease)) error {
	_, _, cancel, err := a.prepareWithCeiling(ctx, bound, nil, onCeiling)
	if err == nil {
		cancel()
	}
	return err
}
