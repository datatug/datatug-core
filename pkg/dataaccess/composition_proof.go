package dataaccess

import (
	"context"
	"fmt"
	"time"

	"github.com/dal-go/dalgo/access"
)

// readComposition is a private, unused-by-Authority composition proof. It only
// accepts the Request.Resources produced by DALgo's secured wrapper. It neither
// walks Query nor resolves names. Inspection cannot use this seam until the
// same authoritative source mapping is available through a supported API.
//
// Each Decide compiles an operation-local ordinary DALgo policy; no compiled
// query policy is retained or reusable for another query. Inputs are copied.
// The existing Authority still refuses object grants and broader query shapes.
type readComposition struct {
	all      bool
	deadline time.Time
	now      func() time.Time
	grants   []compositionGrant
}

type compositionGrant struct {
	deadline time.Time
	objects  []ObjectRef
}

// started must precede the trusted resolve call, as in Authority.authorize.
// This proof checks admission deadlines only. Production transaction/page/idle
// reader guards remain Authority's responsibility and are not enabled here.
func newReadComposition(resolution Resolution, started time.Time, now func() time.Time) (*readComposition, error) {
	connection := resolution.Connection.ID
	if now == nil || started.IsZero() || resolution.Subject.Validate() != nil ||
		!canonical(connection) || !canonical(resolution.Connection.Space) || resolution.Protection.ID != connection ||
		!sameFingerprint(resolution.Connection.Fingerprint, resolution.Protection.Fingerprint) ||
		resolution.ServerTime.IsZero() || !resolution.ValidUntil.After(resolution.ServerTime) ||
		(resolution.Protection.ReadPosture != ReadAll && resolution.Protection.ReadPosture != ReadNone) ||
		(resolution.Protection.AgentRead != AgentInherit && resolution.Protection.AgentRead != AgentNone) {
		return nil, unsupported(connection)
	}
	composition := &readComposition{
		all:      resolution.Protection.ReadPosture == ReadAll,
		deadline: started.Add(min(resolution.ValidUntil.Sub(resolution.ServerTime), time.Minute)), now: now,
	}
	for _, grant := range resolution.Grants {
		if grant.ValidateMVP() != nil || grant.Subject != resolution.Subject || grant.Scope.Connection != connection || resolution.Subject.Kind == Agent {
			return nil, unsupported(connection)
		}
		if grant.State == Approved && grant.ExpiresAt.After(resolution.ServerTime) {
			composition.grants = append(composition.grants, compositionGrant{
				deadline: started.Add(grant.ExpiresAt.Sub(resolution.ServerTime)),
				objects:  append([]ObjectRef(nil), grant.Scope.Objects...),
			})
		}
	}
	if resolution.Subject.Kind == Agent && resolution.Protection.AgentRead == AgentNone {
		composition.all = false
	}
	return composition, nil
}

func (*readComposition) Name() string { return "datatug-read-composition-proof" }

func (p *readComposition) Decide(ctx context.Context, request access.Request) access.Decision {
	now := p.now()
	if !now.Before(p.deadline) || len(request.Resources) == 0 {
		return p.unsupported(request)
	}
	all := p.all
	objects := make(map[ObjectRef]bool)
	for _, grant := range p.grants {
		if !now.Before(grant.deadline) {
			continue
		}
		if len(grant.objects) == 0 {
			all = true
		}
		for _, object := range grant.objects {
			objects[object] = true
		}
	}
	rules := []access.Rule{}
	if all {
		rules = append(rules, access.Root(access.Allow(access.Read, "connection")))
	}
	seen := make(map[access.TableName]bool)
	for _, resource := range request.Resources {
		table, qualified := resource.Table()
		if !qualified {
			// Paths/point keys carry no canonical table identity. Object grants
			// never compile to Collection, whose prefix would include children.
			if resource.Kind() != access.PathResource {
				return p.unsupported(request)
			}
			continue
		}
		if table.Database != "" || !canonical(table.Schema) || !canonical(table.Name) {
			return p.unsupported(request)
		}
		if !seen[table] && (all || objects[ObjectRef{Namespace: table.Schema, Name: table.Name}]) {
			seen[table] = true
			rules = append(rules, access.TableScope(table, access.Allow(access.Read, fmt.Sprintf("table-%d", len(rules)))))
		}
	}
	// Exact byte identities preserve dotted parts, case and literal quote
	// characters. Bare/qualified basename ambiguity remains DALgo's refusal;
	// this proof supplies no invented search-path or case-fold resolution.
	return access.MustPolicy(p.Name(), rules...).Decide(ctx, request)
}

func (p *readComposition) unsupported(request access.Request) access.Decision {
	return access.Decision{Operation: request.Operation, Policy: p.Name(), Code: access.CodeEnforcementUnsupported,
		Explanation: "read composition does not support this request", Effect: "deny"}
}

func (p *readComposition) Authorize(ctx context.Context, request access.Request) error {
	decision := p.Decide(ctx, request)
	if !decision.Allowed {
		return &access.DeniedError{Decision: decision}
	}
	return nil
}
