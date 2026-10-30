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
type Resolution struct {
	Subject    Subject              `json:"subject"`
	Connection ConnectionRef        `json:"connection"`
	Protection ConnectionProtection `json:"protection"`
	Grants     []Grant              `json:"grants"`
	ServerTime time.Time            `json:"serverTime"`
	ValidUntil time.Time            `json:"validUntil"`
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
	if resolve == nil || !canonical(connection.Space) || !canonical(connection.ID) ||
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
// profile currently supports only unqualified, single-source key enumeration.
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
	session := guardedSession{authority: a, read: secured, write: writer}
	return &database{guardedSession: session, secured: secured}, nil
}

type lease struct {
	deadline time.Time
	policy   access.Policy
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
	started := a.now()
	resolution, err := a.resolve(ctx, a.request)
	if err != nil {
		return nil, a.denial(DataRead, "AUTHORITY_UNAVAILABLE")
	}
	if resolution.Subject != a.request.Subject || resolution.Connection.Space != a.request.Connection.Space ||
		resolution.Connection.ID != a.request.Connection.ID || resolution.Protection.ID != a.request.Connection.ID ||
		!sameFingerprint(resolution.Connection.Fingerprint, a.request.Connection.Fingerprint) ||
		!sameFingerprint(resolution.Protection.Fingerprint, a.request.Connection.Fingerprint) {
		return nil, a.denial(DataRead, "BINDING_MISMATCH")
	}
	if resolution.ServerTime.IsZero() || !resolution.ValidUntil.After(resolution.ServerTime) ||
		(resolution.Protection.ReadPosture != ReadAll && resolution.Protection.ReadPosture != ReadNone) ||
		(resolution.Protection.AgentRead != AgentInherit && resolution.Protection.AgentRead != AgentNone) ||
		resolution.Protection.MandatoryFineGrained {
		return nil, a.denial(DataRead, "AUTHORITY_UNSUPPORTED")
	}
	ttl := min(resolution.ValidUntil.Sub(resolution.ServerTime), time.Minute)
	allowed := resolution.Protection.ReadPosture == ReadAll
	var grantTTL time.Duration
	for _, grant := range resolution.Grants {
		if err := grant.ValidateMVP(); err != nil || len(grant.Scope.Objects) != 0 {
			return nil, unsupported(a.request.Connection.ID)
		}
		if grant.Subject != a.request.Subject || grant.Scope.Connection != a.request.Connection.ID || a.request.Subject.Kind == Agent {
			return nil, a.denial(DataRead, "BINDING_MISMATCH")
		}
		if grant.State == Approved && grant.ExpiresAt.After(resolution.ServerTime) {
			grantTTL = max(grantTTL, grant.ExpiresAt.Sub(resolution.ServerTime))
		}
	}
	if !allowed && grantTTL > 0 {
		allowed = true
		ttl = min(ttl, grantTTL)
	}
	if a.request.Subject.Kind == Agent && resolution.Protection.AgentRead == AgentNone {
		allowed = false
	}
	if !allowed {
		return nil, a.denial(DataRead, "READ_DENIED")
	}
	// Starting the interval before the network call subtracts its whole latency.
	// The monotonic local interval does not trust the machine's wall-clock skew.
	return &lease{deadline: started.Add(ttl), policy: readPolicy}, nil
}

func (a *Authority) check(ctx context.Context, authorization *lease) error {
	if !a.now().Before(authorization.deadline) {
		return a.denial(DataRead, "AUTHORIZATION_EXPIRED")
	}
	return ctx.Err()
}

func (a *Authority) prepare(ctx context.Context, bound *lease) (context.Context, *lease, context.CancelFunc, error) {
	if bound != nil {
		if err := a.check(ctx, bound); err != nil {
			return nil, nil, nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	authorization, err := a.authorize(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	if bound != nil && bound.deadline.Before(authorization.deadline) {
		authorization.deadline = bound.deadline
	}
	if err := a.check(ctx, authorization); err != nil {
		return nil, nil, nil, err
	}
	ctx, cancel := context.WithDeadline(ctx, authorization.deadline)
	ctx = context.WithValue(ctx, authorizationKey{}, authorization)
	ctx = access.WithPolicy(ctx, authorization.policy)
	return ctx, authorization, cancel, nil
}

func (a *Authority) recheck(ctx context.Context, bound *lease) error {
	_, _, cancel, err := a.prepare(ctx, bound)
	if err == nil {
		cancel()
	}
	return err
}
